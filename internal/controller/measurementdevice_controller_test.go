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
	"os"
	"reflect"
	"strings"
	"testing"

	chantico "chantico/api/v1alpha1"
	"chantico/internal/filestore"
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
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
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
	previousEnv := config.ValidatedEnv
	config.ValidatedEnv.PodNamespace = ChanticoNamespace
	t.Cleanup(func() { config.ValidatedEnv = previousEnv })

	return &MeasurementDeviceReconciler{
		Client: c,
		Scheme: scheme,
		Jobs:   md.SNMPGeneratorJobBuilder{},
		Files:  filestore.VolumeFileStore{Root: t.TempDir()},
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
	if len(first.Data) != 1 {
		t.Fatal("generator ConfigMap must contain only the generator configuration")
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

func TestEnsureMergedSNMPConfig(t *testing.T) {
	ctx := context.Background()
	r := newReconciler(t)

	if err := EnsureMergedSNMPConfig(ctx, r.Client, r.Client, ChanticoNamespace); err != nil {
		t.Fatalf("EnsureMergedSNMPConfig: %v", err)
	}

	key := types.NamespacedName{Name: md.MergedConfigMapName, Namespace: ChanticoNamespace}
	configMap := &corev1.ConfigMap{}
	if err := r.Get(ctx, key, configMap); err != nil {
		t.Fatalf("get initial merged ConfigMap: %v", err)
	}
	if configMap.Data[md.SnmpConfigKey] == "" {
		t.Fatal("initial merged ConfigMap has no SNMP config")
	}
	var merged snmp.MergedConfig
	if err := yaml.Unmarshal([]byte(configMap.Data[md.SnmpConfigKey]), &merged); err != nil {
		t.Fatalf("unmarshal initial SNMP config: %v", err)
	}
	if len(merged.Auths) != 0 || len(merged.Modules) != 0 {
		t.Fatalf("initial SNMP config should be empty, got %+v", merged)
	}

	configMap.Data[md.SnmpConfigKey] = "existing config"
	if err := r.Update(ctx, configMap); err != nil {
		t.Fatalf("set existing SNMP config: %v", err)
	}
	if err := EnsureMergedSNMPConfig(ctx, r.Client, r.Client, ChanticoNamespace); err != nil {
		t.Fatalf("EnsureMergedSNMPConfig on existing ConfigMap: %v", err)
	}
	if err := r.Get(ctx, key, configMap); err != nil {
		t.Fatalf("get existing merged ConfigMap: %v", err)
	}
	if got := configMap.Data[md.SnmpConfigKey]; got != "existing config" {
		t.Fatalf("existing SNMP config was overwritten: %q", got)
	}
}

func TestCreateGeneratorJob(t *testing.T) {
	ctx := context.Background()
	device := &chantico.MeasurementDevice{ObjectMeta: metav1.ObjectMeta{
		Name: TestMeasurementDeviceName, Namespace: ChanticoNamespace, UID: "device-1", Generation: 7,
	}}
	reconciler := newReconciler(t)
	if result := reconciler.createGeneratorJob(ctx, device); result.Err != nil || result.Action != steps.ActionStop {
		t.Fatalf("create job: %+v", result)
	}
	stored := &batchv1.Job{}
	if err := reconciler.Get(ctx, client.ObjectKeyFromObject(device), stored); err != nil {
		t.Fatal(err)
	}
	if !metav1.IsControlledBy(stored, device) || stored.Annotations[md.GeneratorOutputAnnotation] != md.GeneratorOutputPath(device) {
		t.Fatalf("unexpected stored job: %+v", stored.ObjectMeta)
	}
	if len(stored.Labels) != 0 {
		t.Fatalf("unexpected Job labels: %v", stored.Labels)
	}
	stored.Labels = map[string]string{"external-label": "preserved"}
	stored.Spec.Selector = &metav1.LabelSelector{MatchLabels: map[string]string{batchv1.ControllerUidLabel: "job-uid"}}
	stored.Spec.Template.Labels[batchv1.ControllerUidLabel] = "job-uid"
	if err := reconciler.Update(ctx, stored); err != nil {
		t.Fatal(err)
	}
	if result := reconciler.createGeneratorJob(ctx, device); result.Err != nil || result.Action != steps.ActionStop {
		t.Fatalf("repeated application must succeed: %+v", result)
	}
	job := &batchv1.Job{}
	if err := reconciler.Get(ctx, client.ObjectKeyFromObject(device), job); err != nil {
		t.Fatal(err)
	}
	if job.ResourceVersion != stored.ResourceVersion || !reflect.DeepEqual(job.Spec, stored.Spec) {
		t.Fatal("repeated application must preserve existing Job spec and avoid an update")
	}
	if job.Labels["external-label"] != "preserved" || !metav1.IsControlledBy(job, device) {
		t.Fatal("repeated application must retain external labels and ownership")
	}
	device.Generation++
	if result := reconciler.createGeneratorJob(ctx, device); result.Err != nil || result.Action != steps.ActionStop {
		t.Fatalf("apply existing job: %+v", result)
	}
	if err := reconciler.Get(ctx, client.ObjectKeyFromObject(device), job); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(job.Spec, stored.Spec) || !reflect.DeepEqual(job.Annotations, stored.Annotations) {
		t.Fatal("existing Jobs must not be rewritten or relabeled as a newer generation")
	}

	stored.OwnerReferences[0].Name = "another-device"
	stored.OwnerReferences[0].UID = "another-device"
	if err := reconciler.Update(ctx, stored); err != nil {
		t.Fatal(err)
	}
	var ownerError *controllerutil.AlreadyOwnedError
	if result := reconciler.createGeneratorJob(ctx, device); result.Action != steps.ActionError || !errors.As(result.Err, &ownerError) {
		t.Fatalf("expected ownership conflict: %+v", result)
	}
}

func TestCreateGeneratorJobErrors(t *testing.T) {
	failure := errors.New("job API unavailable")
	for _, test := range []struct {
		name  string
		uid   types.UID
		funcs interceptor.Funcs
	}{
		{
			name: "build error",
		},
		{
			name: "read error",
			uid:  "device-1",
			funcs: interceptor.Funcs{Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
				return failure
			}},
		},
		{
			name: "create error",
			uid:  "device-1",
			funcs: interceptor.Funcs{Create: func(context.Context, client.WithWatch, client.Object, ...client.CreateOption) error {
				return failure
			}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			reconciler := newReconciler(t)
			reconciler.Client = fake.NewClientBuilder().WithScheme(reconciler.Scheme).WithInterceptorFuncs(test.funcs).Build()
			device := &chantico.MeasurementDevice{ObjectMeta: metav1.ObjectMeta{
				Name: TestMeasurementDeviceName, Namespace: ChanticoNamespace, UID: test.uid, Generation: 1,
			}}
			result := reconciler.createGeneratorJob(context.Background(), device)
			if result.Action != steps.ActionError || result.Err == nil {
				t.Fatalf("expected reconciliation error: %+v", result)
			}
			if test.uid != "" && !errors.Is(result.Err, failure) {
				t.Fatalf("must preserve API error: %v", result.Err)
			}
			if len(device.Status.Conditions) != 1 {
				t.Fatalf("expected generation failure condition: %+v", device.Status.Conditions)
			}
			condition := device.Status.Conditions[0]
			if condition.Type != string(chantico.ConditionGenerated) || condition.Status != metav1.ConditionFalse || condition.Reason != string(chantico.ReasonGenerationFailed) {
				t.Fatalf("unexpected failure condition: %+v", condition)
			}
		})
	}
}

func writeGeneratorOutput(t *testing.T, reconciler *MeasurementDeviceReconciler, device *chantico.MeasurementDevice, content string) string {
	t.Helper()
	outputPath := md.GeneratorOutputPath(device)
	if err := reconciler.Files.Write(context.Background(), outputPath, strings.NewReader(content)); err != nil {
		t.Fatal(err)
	}
	return outputPath
}

func TestReconcileSNMPFragment(t *testing.T) {
	measurementDevice := &chantico.MeasurementDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name: TestMeasurementDeviceName, Namespace: ChanticoNamespace,
			UID: types.UID("dev-1"), Generation: 1,
		},
	}
	content := "auths: {foo: {version: 3}}\n"
	key := types.NamespacedName{Name: md.SnmpFragmentConfigMapName(measurementDevice.Name), Namespace: ChanticoNamespace}
	ctx := context.Background()

	r := newReconciler(t)
	outputPath := writeGeneratorOutput(t, r, measurementDevice, content)
	stale := measurementDevice.DeepCopy()
	stale.Generation = 0
	stalePath := writeGeneratorOutput(t, r, stale, "partial previous output")

	if res := r.reconcileSNMPFragment(ctx, measurementDevice); res.Action == steps.ActionError {
		t.Fatalf("first reconcileSNMPFragment errored: %v", res.Err)
	}
	fragment := &corev1.ConfigMap{}
	if err := r.Get(ctx, key, fragment); err != nil {
		t.Fatalf("expected fragment %s: %v", key, err)
	}
	if fragment.Data[md.SnmpConfigKey] != content {
		t.Fatalf("fragment content = %q, want %q", fragment.Data[md.SnmpConfigKey], content)
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
	if want := snmp.Hash([]byte(content)); measurementDevice.Status.ConfigHash != want {
		t.Fatalf("ConfigHash = %q, want %q", measurementDevice.Status.ConfigHash, want)
	}

	for _, filename := range []string{outputPath, stalePath} {
		if _, err := r.Files.ReadAll(ctx, filename); !os.IsNotExist(err) {
			t.Fatalf("generated file %s was not cleaned up: %v", filename, err)
		}
	}
	if res := r.reconcileSNMPFragment(ctx, measurementDevice); res.Action == steps.ActionError {
		t.Fatalf("second reconcileSNMPFragment errored: %v", res.Err)
	}

	// New generation: the fragment is updated.
	measurementDevice.Generation = 2
	content = "auths: {bar: {version: 3}}\n"
	writeGeneratorOutput(t, r, measurementDevice, content)
	if res := r.reconcileSNMPFragment(ctx, measurementDevice); res.Action == steps.ActionError {
		t.Fatalf("third reconcileSNMPFragment errored: %v", res.Err)
	}
	if err := r.Get(ctx, key, fragment); err != nil {
		t.Fatal(err)
	}
	if fragment.Data[md.SnmpConfigKey] != content || fragment.Annotations[md.GenerationAnnotation] != "2" {
		t.Fatalf("fragment not updated: %v %v", fragment.Data, fragment.Annotations)
	}
}

func TestReconcileSNMPFragmentFailures(t *testing.T) {
	measurementDevice := &chantico.MeasurementDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name: TestMeasurementDeviceName, Namespace: ChanticoNamespace,
			UID: types.UID("dev-1"), Generation: 1,
		},
	}
	tests := []struct {
		name       string
		content    string
		uid        types.UID
		generation int64
	}{
		{"missing file", "", "", 0},
		{"empty file", " \n", "dev-1", 1},
		{"invalid yaml", "auths: [\n", "dev-1", 1},
		{"stale generation", "auths: {}\n", "dev-1", 0},
		{"foreign device", "auths: {}\n", "another-device", 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			device := measurementDevice.DeepCopy()
			previous := snmpFragment(device.Name, device.Namespace, "auths: {previous: {version: 3}}\n")
			previous.Annotations = map[string]string{md.GenerationAnnotation: "0"}
			reconciler := newReconciler(t, previous)
			if test.uid != "" {
				source := device.DeepCopy()
				source.UID = test.uid
				source.Generation = test.generation
				writeGeneratorOutput(t, reconciler, source, test.content)
			}

			if res := reconciler.reconcileSNMPFragment(context.Background(), device); res.Action != steps.ActionError {
				t.Fatalf("expected error, got %v", res.Action)
			}
			stored := &corev1.ConfigMap{}
			if err := reconciler.Get(context.Background(), client.ObjectKeyFromObject(previous), stored); err != nil {
				t.Fatal(err)
			}
			if stored.Data[md.SnmpConfigKey] != previous.Data[md.SnmpConfigKey] || stored.Annotations[md.GenerationAnnotation] != "0" {
				t.Fatal("failed generation must preserve previous active fragment")
			}
		})
	}
}

type configMapCreateErrorClient struct {
	client.Client
	failure error
}

func (failing configMapCreateErrorClient) Create(context.Context, client.Object, ...client.CreateOption) error {
	return failing.failure
}

func TestReconcileSNMPFragmentRetry(t *testing.T) {
	ctx := context.Background()
	device := &chantico.MeasurementDevice{ObjectMeta: metav1.ObjectMeta{
		Name: TestMeasurementDeviceName, Namespace: ChanticoNamespace, UID: "device-1", Generation: 1,
	}}
	reconciler := newReconciler(t)
	content := "auths: {tno: {version: 3}}\n"
	outputPath := writeGeneratorOutput(t, reconciler, device, content)
	kubeClient := reconciler.Client
	failure := errors.New("ConfigMap write unavailable")
	reconciler.Client = configMapCreateErrorClient{Client: kubeClient, failure: failure}
	if result := reconciler.reconcileSNMPFragment(ctx, device); !errors.Is(result.Err, failure) {
		t.Fatalf("expected persistence error, got %+v", result)
	}
	if output, err := reconciler.Files.ReadAll(ctx, outputPath); err != nil || string(output) != content {
		t.Fatalf("failed persistence must retain generated output: %q, %v", output, err)
	}
	reconciler.Client = kubeClient
	if result := reconciler.reconcileSNMPFragment(ctx, device); result.Err != nil {
		t.Fatalf("retry after API recovery: %v", result.Err)
	}
	if _, err := reconciler.Files.ReadAll(ctx, outputPath); !os.IsNotExist(err) {
		t.Fatalf("successful persistence must clean generated output: %v", err)
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
	outputPath := writeGeneratorOutput(t, r, measurementDevice, "unfinished output")
	otherDevice := measurementDevice.DeepCopy()
	otherDevice.UID = "other-device"
	otherPath := writeGeneratorOutput(t, r, otherDevice, "other output")

	// Start deletion of measurementDevice.
	res := r.reconcileDeletion(context.Background(), measurementDevice)
	if res.Action == steps.ActionError {
		t.Fatalf("reconcileDeletion errored: %v", res.Err)
	}
	if res.Action != steps.ActionStop {
		t.Fatalf("expected Stop, got %v", res.Action)
	}
	if !controllerutil.ContainsFinalizer(measurementDevice, chantico.SNMPUpdateFinalizer) {
		t.Fatal("finalizer must remain until job deletion completes")
	}
	if _, err := r.Files.ReadAll(context.Background(), outputPath); err != nil {
		t.Fatalf("output removed before job deletion completes: %v", err)
	}
	if res = r.reconcileDeletion(context.Background(), measurementDevice); res.Action == steps.ActionError {
		t.Fatalf("finish deletion: %v", res.Err)
	}
	if _, err := r.Files.ReadAll(context.Background(), outputPath); !os.IsNotExist(err) {
		t.Fatalf("deleted device output remains: %v", err)
	}
	if _, err := r.Files.ReadAll(context.Background(), otherPath); err != nil {
		t.Fatalf("other device output was removed: %v", err)
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
