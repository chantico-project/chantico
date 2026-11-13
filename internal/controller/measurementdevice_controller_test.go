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
	"errors"
	"strings"
	"testing"

	chantico "chantico/api/v1alpha1"
	ch "chantico/internal/k8s"
	"chantico/internal/snmp"
	"chantico/internal/steps"

	config "chantico/internal/configuration"
	md "chantico/internal/measurementdevice"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	yaml "go.yaml.in/yaml/v2"
)

const (
	TestMeasurementDeviceName = "tno"
	ChanticoNamespace         = "chantico"
)

func newReconciler(t *testing.T, objs ...runtime.Object) *MeasurementDeviceReconciler {
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
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(objs...).
		Build()

	t.Setenv(config.ChanticoNamespaceEnv, ChanticoNamespace)

	return &MeasurementDeviceReconciler{
		Client: c,
		Scheme: scheme,
	}
}

func TestReconcileGeneratorConfigMap(t *testing.T) {
	measurementDevice := &chantico.MeasurementDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name: TestMeasurementDeviceName, Namespace: ChanticoNamespace,
			UID: types.UID("dev-1"),
		},
		Spec: chantico.MeasurementDeviceSpec{
			AuthFrom: &ch.SecretConfigMapSelector{Value: ""},
			Walks:    []string{"1.3.6.1"},
		},
	}
	r := newReconciler(t, measurementDevice)
	key := types.NamespacedName{Name: md.GeneratorConfigMapName(measurementDevice.Name), Namespace: ChanticoNamespace}

	// First run: creates the ConfigMap, owned by the device.
	res := r.reconcileGeneratorConfigMap(context.Background(), measurementDevice)
	if res.Action == steps.ActionError {
		t.Fatalf("first reconcileGeneratorConfigMap errored: %v", res.Err)
	}
	first := &corev1.ConfigMap{}
	if err := r.Get(context.Background(), key, first); err != nil {
		t.Fatalf("expected ConfigMap %s: %v", key, err)
	}
	if first.Data[md.GeneratorConfigKey] == "" {
		t.Fatalf("ConfigMap has no %q entry: %+v", md.GeneratorConfigKey, first.Data)
	}
	if !metav1.IsControlledBy(first, measurementDevice) {
		t.Fatalf("ConfigMap is not controlled by the measurement device: %+v", first.OwnerReferences)
	}

	// Second run should not change anything.
	res = r.reconcileGeneratorConfigMap(context.Background(), measurementDevice)
	if res.Action == steps.ActionError {
		t.Fatalf("second reconcileGeneratorConfigMap errored: %v", res.Err)
	}
	second := &corev1.ConfigMap{}
	if err := r.Get(context.Background(), key, second); err != nil {
		t.Fatal(err)
	}
	if first.ResourceVersion != second.ResourceVersion {
		t.Fatalf("ConfigMap changed on idempotent run: %q -> %q", first.ResourceVersion, second.ResourceVersion)
	}

	// A changed spec updates the ConfigMap.
	measurementDevice.Spec.Walks = []string{"1.3.6.1", "1.3.6.2"}
	if res = r.reconcileGeneratorConfigMap(context.Background(), measurementDevice); res.Action == steps.ActionError {
		t.Fatalf("third reconcileGeneratorConfigMap errored: %v", res.Err)
	}
	third := &corev1.ConfigMap{}
	if err := r.Get(context.Background(), key, third); err != nil {
		t.Fatal(err)
	}
	if third.Data[md.GeneratorConfigKey] == second.Data[md.GeneratorConfigKey] {
		t.Fatalf("ConfigMap not updated after spec change: %q", third.Data[md.GeneratorConfigKey])
	}
}

func snmpFragment(name, namespace string, content string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      md.SnmpFragmentConfigMapName(name),
			Namespace: namespace,
			Labels:    map[string]string{md.SnmpFragmentLabel: "true"},
		},
		Data: map[string]string{md.SnmpConfigKey: content},
	}
}

func TestReconcileMergedSNMPFile(t *testing.T) {
	r := newReconciler(t,
		snmpFragment("a", ChanticoNamespace, "auths: {foo: {version: 3}}\nmodules: {foo: {walk: [1.3]}}\n"),
		snmpFragment("b", ChanticoNamespace, "auths: {bar: {version: 3}}\nmodules: {bar: {walk: [1.4]}}\n"),
		// Not a fragment: must not end up in the merged config.
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "a-generator", Namespace: ChanticoNamespace},
			Data:       map[string]string{md.GeneratorConfigKey: "auths: {other: {version: 3}}\n"},
		},
	)

	measurementDevice := &chantico.MeasurementDevice{ObjectMeta: metav1.ObjectMeta{Name: TestMeasurementDeviceName, Namespace: ChanticoNamespace}}
	if res := r.reconcileMergedSNMPFile(context.Background(), measurementDevice); res.Action == steps.ActionError {
		t.Fatalf("reconcileMergedSNMPFile errored: %v", res.Err)
	}

	key := types.NamespacedName{Name: md.MergedConfigMapName, Namespace: ChanticoNamespace}
	merged := &corev1.ConfigMap{}
	if err := r.Get(context.Background(), key, merged); err != nil {
		t.Fatalf("expected merged ConfigMap %s: %v", key, err)
	}
	var out snmp.MergedConfig
	if err := yaml.Unmarshal([]byte(merged.Data[md.SnmpConfigKey]), &out); err != nil {
		t.Fatalf("unmarshal merged config: %v", err)
	}
	for _, name := range []string{"foo", "bar"} {
		if _, ok := out.Auths[name]; !ok {
			t.Errorf("auth %q missing from merged config: %v", name, out.Auths)
		}
	}
	if _, ok := out.Auths["other"]; ok {
		t.Errorf("generator config leaked into merged config: %v", out.Auths)
	}

	// The reload hash matches the merged content.
	want, err := r.mergedSNMPConfig(context.Background(), measurementDevice)
	if err != nil {
		t.Fatal(err)
	}
	if snmp.Hash([]byte(merged.Data[md.SnmpConfigKey])) != snmp.Hash(want) {
		t.Fatalf("merged ConfigMap content does not match mergedSNMPConfig output")
	}
}

type fakeJobLogs struct {
	output string
	err    error
	calls  int
}

func (f *fakeJobLogs) GetJobLogs(_ context.Context, _ *batchv1.Job) ([]byte, error) {
	f.calls++
	return []byte(f.output), f.err
}

func TestReconcileSNMPFragment(t *testing.T) {
	measurementDevice := &chantico.MeasurementDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name: TestMeasurementDeviceName, Namespace: ChanticoNamespace,
			UID: types.UID("dev-1"), Generation: 1,
		},
	}
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tno-generator",
			Namespace: ChanticoNamespace,
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
	key := types.NamespacedName{Name: md.SnmpFragmentConfigMapName(measurementDevice.Name), Namespace: ChanticoNamespace}
	ctx := context.Background()

	r := newReconciler(t, job)
	logs := &fakeJobLogs{output: "auths: {foo: {version: 3}}\n"}
	r.Logs = logs

	// First run: the fragment is created from the Job log.
	if res := r.reconcileSNMPFragment(ctx, measurementDevice); res.Action == steps.ActionError {
		t.Fatalf("first reconcileSNMPFragment errored: %v", res.Err)
	}
	fragment := &corev1.ConfigMap{}
	if err := r.Get(ctx, key, fragment); err != nil {
		t.Fatalf("expected fragment %s: %v", key, err)
	}
	if fragment.Data[md.SnmpConfigKey] != logs.output {
		t.Fatalf("fragment content = %q, want %q", fragment.Data[md.SnmpConfigKey], logs.output)
	}
	if fragment.Labels[md.SnmpFragmentLabel] != "true" {
		t.Fatalf("fragment misses label %s: %v", md.SnmpFragmentLabel, fragment.Labels)
	}
	if fragment.Annotations[md.GenerationAnnotation] != "1" {
		t.Fatalf("fragment generation = %q, want 1", fragment.Annotations[md.GenerationAnnotation])
	}
	if !metav1.IsControlledBy(fragment, measurementDevice) {
		t.Fatalf("fragment is not controlled by the measurement device: %+v", fragment.OwnerReferences)
	}
	if want := snmp.Hash([]byte(logs.output)); measurementDevice.Status.ConfigHash != want {
		t.Fatalf("ConfigHash = %q, want %q", measurementDevice.Status.ConfigHash, want)
	}

	// Same generation: the log is not read again.
	if res := r.reconcileSNMPFragment(ctx, measurementDevice); res.Action == steps.ActionError {
		t.Fatalf("second reconcileSNMPFragment errored: %v", res.Err)
	}
	if logs.calls != 1 {
		t.Fatalf("job log read %d times, want 1", logs.calls)
	}

	// New generation: the fragment is updated.
	measurementDevice.Generation = 2
	logs.output = "auths: {bar: {version: 3}}\n"
	if res := r.reconcileSNMPFragment(ctx, measurementDevice); res.Action == steps.ActionError {
		t.Fatalf("third reconcileSNMPFragment errored: %v", res.Err)
	}
	if err := r.Get(ctx, key, fragment); err != nil {
		t.Fatal(err)
	}
	if fragment.Data[md.SnmpConfigKey] != logs.output || fragment.Annotations[md.GenerationAnnotation] != "2" {
		t.Fatalf("fragment not updated: %v %v", fragment.Data, fragment.Annotations)
	}
	if logs.calls != 2 {
		t.Fatalf("job log read %d times, want 2", logs.calls)
	}
}

func TestReconcileSNMPFragmentFailures(t *testing.T) {
	measurementDevice := &chantico.MeasurementDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name: TestMeasurementDeviceName, Namespace: ChanticoNamespace,
			UID: types.UID("dev-1"), Generation: 1,
		},
	}
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tno-generator",
			Namespace: ChanticoNamespace,
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

	tests := []struct {
		name string
		objs []runtime.Object
		logs *fakeJobLogs
	}{
		{"no job", nil, &fakeJobLogs{output: "auths: {}\n"}},
		{"log unavailable", []runtime.Object{job}, &fakeJobLogs{err: errors.New("no pod")}},
		{"empty log", []runtime.Object{job}, &fakeJobLogs{output: " \n"}},
		{"invalid yaml", []runtime.Object{job}, &fakeJobLogs{output: "auths: [\n"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newReconciler(t, tt.objs...)
			r.Logs = tt.logs
			device := measurementDevice.DeepCopy()

			if res := r.reconcileSNMPFragment(context.Background(), device); res.Action != steps.ActionError {
				t.Fatalf("expected error, got %v", res.Action)
			}
			key := types.NamespacedName{Name: md.SnmpFragmentConfigMapName(device.Name), Namespace: ChanticoNamespace}
			if err := r.Get(context.Background(), key, &corev1.ConfigMap{}); !apierrors.IsNotFound(err) {
				t.Fatalf("expected no fragment, got err=%v", err)
			}
		})
	}
}

func TestReconcileDeletion(t *testing.T) {
	now := metav1.Now()
	measurementDevice := &chantico.MeasurementDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:              TestMeasurementDeviceName,
			Namespace:         ChanticoNamespace,
			UID:               types.UID("dev-del"),
			DeletionTimestamp: &now,
			Finalizers:        []string{chantico.SNMPUpdateFinalizer},
		},
		Spec: chantico.MeasurementDeviceSpec{
			AuthFrom: &ch.SecretConfigMapSelector{Value: ""},
			Walks:    []string{"1.3.6.1"},
		},
	}

	// Owned job that must be deleted.
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tno-generator",
			Namespace: ChanticoNamespace,
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
		ObjectMeta: metav1.ObjectMeta{Name: "chantico-snmp", Namespace: ChanticoNamespace},
	}
	// Fragment of the device and of another device.
	fragment := snmpFragment(measurementDevice.Name, ChanticoNamespace, "auths: {gone: {version: 3}}\n")
	fragment.OwnerReferences = job.OwnerReferences
	other := snmpFragment("other", ChanticoNamespace, "auths: {kept: {version: 3}}\n")
	r := newReconciler(t, measurementDevice, job, exporter, fragment, other)

	// Start deletion of measurementDevice.
	res := r.reconcileDeletion(context.Background(), measurementDevice)
	if res.Action == steps.ActionError {
		t.Fatalf("reconcileDeletion errored: %v", res.Err)
	}
	if res.Action != steps.ActionStop {
		t.Fatalf("expected Stop, got %v", res.Action)
	}

	// Fragment must be removed and left out of the merged config.
	err := r.Get(context.Background(), client.ObjectKeyFromObject(fragment), &corev1.ConfigMap{})
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected fragment to be deleted, got err=%v", err)
	}
	merged := &corev1.ConfigMap{}
	if err := r.Get(context.Background(), types.NamespacedName{Name: md.MergedConfigMapName, Namespace: ChanticoNamespace}, merged); err != nil {
		t.Fatalf("expected merged ConfigMap: %v", err)
	}
	if got := merged.Data[md.SnmpConfigKey]; strings.Contains(got, "gone") || !strings.Contains(got, "kept") {
		t.Fatalf("unexpected merged config after deletion:\n%s", got)
	}

	// Owned job must be deleted.
	got := &batchv1.Job{}
	err = r.Get(context.Background(),
		types.NamespacedName{Name: job.Name, Namespace: job.Namespace}, got)
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected owned job to be deleted, got err=%v", err)
	}

	// Finalizer must have been removed.
	if controllerutil.ContainsFinalizer(measurementDevice, chantico.SNMPUpdateFinalizer) {
		t.Fatalf("expected finalizer %q to be removed", chantico.SNMPUpdateFinalizer)
	}
}
