/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	chantico "chantico/api/v1alpha1"
	config "chantico/internal/configuration"
	md "chantico/internal/measurementdevice"
	"chantico/internal/prometheus"
	"chantico/internal/snmp"
	"chantico/internal/steps"

	yaml "go.yaml.in/yaml/v2"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func newReconciler(t *testing.T, root string, objs ...runtime.Object) *MeasurementDeviceReconciler {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := chantico.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(objs...).
		Build()

	// Take first object with a namespace as reconciler namespace
	namespace := ""
	for _, obj := range objs {
		accessor, err := meta.Accessor(obj)
		if err != nil {
			t.Fatal(err)
		}
		namespace = accessor.GetNamespace()
		if namespace != "" {
			break
		}
	}

	return &MeasurementDeviceReconciler{
		Client:    c,
		Scheme:    scheme,
		Paths:     md.NewPaths(root),
		Namespace: namespace,
	}
}

func TestWriteReconcileGeneratorFile(t *testing.T) {
	root := t.TempDir()
	measurementDevice := &chantico.MeasurementDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name: "tno", Namespace: "chantico",
			UID: types.UID("dev-1"),
		},
		Spec: chantico.MeasurementDeviceSpec{
			Auth:  snmp.GeneratorAuth{},
			Walks: []string{"1.3.6.1"},
		},
	}
	r := newReconciler(t, root, measurementDevice)

	// First run: writes the file.
	res := r.reconcileGeneratorFile(context.Background(), measurementDevice)
	if res.Action == steps.ActionError {
		t.Fatalf("first reconcileGeneratorFile errored: %v", res.Err)
	}
	path := r.Paths.GeneratorFile(measurementDevice.GetUID())
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected file %s: %v", path, err)
	}

	// Second run should not apply any changes.
	res = r.reconcileGeneratorFile(context.Background(), measurementDevice)
	if res.Action == steps.ActionError {
		t.Fatalf("second reconcileGeneratorFile errored: %v", res.Err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after second run: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("file changed on idempotent run:\nfirst:  %q\nsecond: %q", first, second)
	}
}

func TestWriteReconcileMergedSNMPFile(t *testing.T) {
	root := t.TempDir()
	r := newReconciler(t, root)

	// Seed two per-device files.
	if err := os.MkdirAll(r.Paths.SNMPDir(), 0777); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(r.Paths.SNMPDir(), "snmp-a.yaml"),
		[]byte("auths: {foo: {version: 3}}\nmodules: {foo: {walk: [1.3]}}\n"))
	writeFile(t, filepath.Join(r.Paths.SNMPDir(), "snmp-b.yaml"),
		[]byte("auths: {bar: {version: 3}}\nmodules: {bar: {walk: [1.4]}}\n"))

	measurementDevice := &chantico.MeasurementDevice{ObjectMeta: metav1.ObjectMeta{Name: "tno", Namespace: "chantico"}}
	if res := r.reconcileMergedSNMPFile(context.Background(), measurementDevice); res.Action == steps.ActionError {
		t.Fatalf("reconcileMergedSNMPFile errored: %v", res.Err)
	}

	got, err := os.ReadFile(r.Paths.MergedSNMPFile())
	if err != nil {
		t.Fatalf("read merged file: %v", err)
	}
	merged, err := snmp.GetMergedSortedSNMPConfig(r.Paths.SNMPDir())
	if err != nil {
		t.Fatalf("get merged config: %v", err)
	}

	wantHash := snmp.Hash(merged)
	if snmp.Hash(got) != wantHash {
		t.Fatalf("merged file content does not match GetMergedSortedSNMPConfig output")
	}

	// No leftover .tmp file from the atomic rename.
	if _, err := os.Stat(r.Paths.MergedSNMPFile() + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temp file leaked: %v", err)
	}
}

func writeFile(t *testing.T, path string, b []byte) {
	t.Helper()
	if err := os.WriteFile(path, b, 0777); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestReconcileDeletion(t *testing.T) {
	root := t.TempDir()

	now := metav1.Now()
	measurementDevice := &chantico.MeasurementDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "tno",
			Namespace:         "chantico",
			UID:               types.UID("dev-del"),
			DeletionTimestamp: &now,
			Finalizers:        []string{chantico.SNMPUpdateFinalizer},
		},
		Spec: chantico.MeasurementDeviceSpec{
			Auth:  snmp.GeneratorAuth{},
			Walks: []string{"1.3.6.1"},
		},
	}

	// Owned job that must be deleted.
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tno-generator",
			Namespace: "chantico",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: chantico.GroupVersion.String(),
					Kind:       "MeasurementDevice",
					Name:       measurementDevice.Name,
					UID:        measurementDevice.UID,
					Controller: ptr.To(true),
				},
			},
		},
	}

	exporter := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "chantico-snmp", Namespace: "chantico"},
	}
	r := newReconciler(t, root, measurementDevice, job, exporter)

	// Seed the per-device files that deletion should remove.
	if err := os.MkdirAll(r.Paths.SNMPDir(), 0777); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(r.Paths.GeneratorFile(measurementDevice.UID)), 0777); err != nil {
		t.Fatal(err)
	}
	writeFile(t, r.Paths.GeneratorFile(measurementDevice.UID), []byte("auths: {}\n"))
	writeFile(t, r.Paths.SNMPFile(measurementDevice.UID), []byte("auths: {}\nmodules: {}\n"))

	// Create scrape config and targets dir to check that they are deleted correctly
	if err := os.MkdirAll(r.Paths.ScrapeConfigsDir(), 0777); err != nil {
		t.Fatal(err)
	}
	writeFile(t, r.Paths.ScrapeConfigFile(measurementDevice.UID), []byte("scrape_configs: []\n"))
	targetsDir := r.Paths.TargetsDir(measurementDevice.UID)
	if err := os.MkdirAll(targetsDir, 0777); err != nil {
		t.Fatal(err)
	}
	writeFile(t, r.Paths.TargetsFile(measurementDevice.UID, measurementDevice.UID), []byte("[]"))

	// Start deletion of measurementDevice.
	res := r.reconcileDeletion(context.Background(), measurementDevice)
	if res.Action == steps.ActionError {
		t.Fatalf("reconcileDeletion errored: %v", res.Err)
	}
	if res.Action != steps.ActionStop {
		t.Fatalf("expected Stop, got %v", res.Action)
	}

	// Generator, SNMP and scrape config files must be removed.
	for _, p := range []string{
		r.Paths.GeneratorFile(measurementDevice.UID),
		r.Paths.SNMPFile(measurementDevice.UID),
		r.Paths.ScrapeConfigFile(measurementDevice.UID),
	} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be removed, stat err = %v", p, err)
		}
	}

	// Service discovery targets dir must be removed.
	if _, err := os.Stat(targetsDir); !os.IsNotExist(err) {
		t.Fatalf("expected targets dir %s to be removed, stat err = %v", targetsDir, err)
	}

	// Owned job must be deleted.
	got := &batchv1.Job{}
	err := r.Get(context.Background(),
		types.NamespacedName{Name: job.Name, Namespace: job.Namespace}, got)
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected owned job to be deleted, got err=%v", err)
	}

	// Finalizer must have been removed.
	if controllerutil.ContainsFinalizer(measurementDevice, chantico.SNMPUpdateFinalizer) {
		t.Fatalf("expected finalizer %q to be removed", chantico.SNMPUpdateFinalizer)
	}
}

func TestReconcileScrapeJobFile_Prometheus(t *testing.T) {
	root := t.TempDir()
	measurementDevice := &chantico.MeasurementDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "node-exporter", Namespace: "chantico", UID: types.UID("prom-1")},
		Spec: chantico.MeasurementDeviceSpec{
			Type:       chantico.MeasurementDeviceTypePrometheus,
			Prometheus: chantico.PrometheusConfig{Path: "/metrics"},
		},
	}
	r := newReconciler(t, root, measurementDevice)

	res := r.reconcileScrapeJobFile(context.Background(), measurementDevice)
	if res.Action == steps.ActionError {
		t.Fatalf("reconcileScrapeJobFile errored: %v", res.Err)
	}

	path := r.Paths.ScrapeConfigFile(measurementDevice.GetUID())
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected scrape config file %s: %v", path, err)
	}

	var parsed prometheus.ScrapeConfigFile
	if err := yaml.Unmarshal(got, &parsed); err != nil {
		t.Fatalf("unmarshal scrape config: %v", err)
	}
	if len(parsed.ScrapeConfigs) != 1 {
		t.Fatalf("expected 1 scrape config, got %d", len(parsed.ScrapeConfigs))
	}
	sc := parsed.ScrapeConfigs[0]
	if sc.JobName != "node-exporter" {
		t.Errorf("expected job_name %q, got %q", "node-exporter", sc.JobName)
	}
	if sc.MetricsPath != "/metrics" {
		t.Errorf("expected metrics_path %q, got %q", "/metrics", sc.MetricsPath)
	}
	if len(sc.RelabelConfigs) != 0 {
		t.Errorf("expected no relabel_configs for prometheus type, got %d", len(sc.RelabelConfigs))
	}

	// Targets directory must have been created for service discovery.
	if info, err := os.Stat(r.Paths.TargetsDir(measurementDevice.GetUID())); err != nil || !info.IsDir() {
		t.Fatalf("expected targets dir to exist: %v", err)
	}

	// Second run should not have any effect since the file is the same
	res = r.reconcileScrapeJobFile(context.Background(), measurementDevice)
	if res.Action == steps.ActionError {
		t.Fatalf("second reconcileScrapeJobFile errored: %v", res.Err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after second run: %v", err)
	}
	if string(got) != string(second) {
		t.Fatalf("file changed on idempotent run:\nfirst:  %q\nsecond: %q", got, second)
	}

	cond := meta.FindStatusCondition(measurementDevice.Status.Conditions, string(chantico.ConditionConfig))
	if cond == nil || cond.Status != metav1.ConditionTrue {
		t.Fatalf("expected ConditionConfig to be true, got %+v", cond)
	}
}

func TestReconcileScrapeJobFile_SNMP(t *testing.T) {
	config.ValidatedEnv.SnmpExporterServiceHost = "chantico-snmp"
	config.ValidatedEnv.SnmpExporterServicePort = "9116"
	t.Cleanup(func() {
		config.ValidatedEnv.SnmpExporterServiceHost = ""
		config.ValidatedEnv.SnmpExporterServicePort = ""
	})

	root := t.TempDir()
	measurementDevice := &chantico.MeasurementDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "tno", Namespace: "chantico", UID: types.UID("snmp-1")},
		Spec: chantico.MeasurementDeviceSpec{
			Type:  chantico.MeasurementDeviceTypeSNMP,
			Auth:  snmp.GeneratorAuth{},
			Walks: []string{"1.3.6.1"},
		},
	}
	r := newReconciler(t, root, measurementDevice)

	res := r.reconcileScrapeJobFile(context.Background(), measurementDevice)
	if res.Action == steps.ActionError {
		t.Fatalf("reconcileScrapeJobFile errored: %v", res.Err)
	}

	got, err := os.ReadFile(r.Paths.ScrapeConfigFile(measurementDevice.GetUID()))
	if err != nil {
		t.Fatalf("expected scrape config file: %v", err)
	}
	var parsed prometheus.ScrapeConfigFile
	if err := yaml.Unmarshal(got, &parsed); err != nil {
		t.Fatalf("unmarshal scrape config: %v", err)
	}
	sc := parsed.ScrapeConfigs[0]
	if sc.MetricsPath != "/snmp" {
		t.Errorf("expected metrics_path %q, got %q", "/snmp", sc.MetricsPath)
	}
	wantReplacement := "chantico-snmp:9116"
	found := false
	for _, rc := range sc.RelabelConfigs {
		if rc.TargetLabel == "__address__" && rc.Replacement == wantReplacement {
			found = true
		}
	}
	if !found {
		t.Errorf("expected relabel_config replacing __address__ with %q, got %+v", wantReplacement, sc.RelabelConfigs)
	}
}

func TestCreateScrapeConfig_UnsupportedType(t *testing.T) {
	measurementDevice := &chantico.MeasurementDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "bad"},
		Spec:       chantico.MeasurementDeviceSpec{Type: "unknown"},
	}
	if _, err := createScrapeConfig(measurementDevice); err == nil {
		t.Fatal("expected error for unsupported measurement device type")
	}
}
