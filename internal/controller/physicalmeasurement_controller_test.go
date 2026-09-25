/*
Copyright 2025-2026.

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
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	chantico "chantico/api/v1alpha1"
	config "chantico/internal/configuration"
	"chantico/internal/filestore"
	pm "chantico/internal/physicalmeasurement"
	"chantico/internal/steps"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func setupPhysicalMeasurementTest(t *testing.T, objs ...runtime.Object) (string, *PhysicalMeasurementReconciler) {
	t.Helper()

	tmpDir := t.TempDir()
	t.Setenv(config.ChanticoVolumeLocationEnv, tmpDir)
	config.ValidatedEnv, _ = config.ValidateEnv()

	targetsDir := filepath.Join(tmpDir, prometheusTargetsDir)
	if err := os.MkdirAll(targetsDir, 0755); err != nil {
		t.Fatalf("create targets directory %s: %v", targetsDir, err)
	}

	scheme := runtime.NewScheme()
	if err := chantico.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(objs...).
		Build()

	return tmpDir, &PhysicalMeasurementReconciler{Client: client, Scheme: scheme}
}

func TestReconcileTargetFile_WritesTargetFile(t *testing.T) {
	tmpDir, reconciler := setupPhysicalMeasurementTest(t)

	physicalMeasurement := &chantico.PhysicalMeasurement{
		ObjectMeta: metav1.ObjectMeta{
			Name: "physical-measurement",
			UID:  types.UID("18ac6360-39e7-4ee3-a9b8-58992958e29a"),
		},
		Spec: chantico.PhysicalMeasurementSpec{
			MeasurementDevice: "device-a",
			Ip:                "192.168.1.10",
		},
	}

	res := reconciler.reconcileTargetFile(t.Context(), physicalMeasurement)
	if res.Action == steps.ActionError {
		t.Fatalf("reconcileTargetFile errored: %v", res.Err)
	}
	if res.Action != steps.ActionContinue {
		t.Fatalf("expected Continue, got %v", res.Action)
	}

	targetsDir := filepath.Join(tmpDir, prometheusTargetsDir)
	targets := readFileSDTargets(t, filepath.Join(targetsDir, "physical-measurement.json"))
	if len(targets) != 1 {
		t.Fatalf("expected 1 target group, got %d", len(targets))
	}
	if got := targets[0].Labels["__param_module"]; got != physicalMeasurement.Spec.MeasurementDevice {
		t.Errorf("expected module label %q, got %q", physicalMeasurement.Spec.MeasurementDevice, got)
	}
	if want := []string{physicalMeasurement.Spec.Ip}; !slices.Equal(targets[0].Targets, want) {
		t.Errorf("expected targets %v, got %v", want, targets[0].Targets)
	}

	assertTargetFiles(t, targetsDir, []string{"physical-measurement.json"})
	assertPhysicalMeasurementCondition(t, physicalMeasurement, chantico.ConditionApplied, metav1.ConditionTrue, chantico.ReasonReconciled)
}

func TestReconcileTargetFile_SkipsWriteWhenUnchanged(t *testing.T) {
	tmpDir, reconciler := setupPhysicalMeasurementTest(t)

	physicalMeasurement := &chantico.PhysicalMeasurement{
		ObjectMeta: metav1.ObjectMeta{Name: "physical-measurement"},
		Spec: chantico.PhysicalMeasurementSpec{
			MeasurementDevice: "device-a",
			Ip:                "192.168.1.10",
		},
	}

	if res := reconciler.reconcileTargetFile(t.Context(), physicalMeasurement); res.Action == steps.ActionError {
		t.Fatalf("first reconcileTargetFile errored: %v", res.Err)
	}

	targetPath := filepath.Join(tmpDir, prometheusTargetsDir, "physical-measurement.json")
	before, err := os.Stat(targetPath)
	if err != nil {
		t.Fatalf("stat target file %s: %v", targetPath, err)
	}

	if res := reconciler.reconcileTargetFile(t.Context(), physicalMeasurement); res.Action == steps.ActionError {
		t.Fatalf("second reconcileTargetFile errored: %v", res.Err)
	}

	after, err := os.Stat(targetPath)
	if err != nil {
		t.Fatalf("stat target file %s: %v", targetPath, err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Errorf("expected target file not to be rewritten, mod time changed from %v to %v", before.ModTime(), after.ModTime())
	}
	assertPhysicalMeasurementCondition(t, physicalMeasurement, chantico.ConditionApplied, metav1.ConditionTrue, chantico.ReasonReconciled)
}

func TestReconcileTargetFile_CreatesMissingTargetsDir(t *testing.T) {
	tmpDir, reconciler := setupPhysicalMeasurementTest(t)

	targetsDir := filepath.Join(tmpDir, prometheusTargetsDir)
	if err := os.RemoveAll(targetsDir); err != nil {
		t.Fatalf("remove targets directory %s: %v", targetsDir, err)
	}

	physicalMeasurement := &chantico.PhysicalMeasurement{
		ObjectMeta: metav1.ObjectMeta{Name: "physical-measurement"},
		Spec: chantico.PhysicalMeasurementSpec{
			MeasurementDevice: "device-a",
			Ip:                "192.168.1.10",
		},
	}

	if res := reconciler.reconcileTargetFile(t.Context(), physicalMeasurement); res.Action == steps.ActionError {
		t.Fatalf("reconcileTargetFile errored: %v", res.Err)
	}

	assertTargetFiles(t, targetsDir, []string{"physical-measurement.json"})
}

func TestReconcileTargetFile_OverwritesExisting(t *testing.T) {
	tmpDir, reconciler := setupPhysicalMeasurementTest(t)

	physicalMeasurement := &chantico.PhysicalMeasurement{
		ObjectMeta: metav1.ObjectMeta{Name: "physical-measurement"},
		Spec: chantico.PhysicalMeasurementSpec{
			MeasurementDevice: "device-a",
			Ip:                "192.168.1.10",
		},
	}

	if res := reconciler.reconcileTargetFile(t.Context(), physicalMeasurement); res.Action == steps.ActionError {
		t.Fatalf("first reconcileTargetFile errored: %v", res.Err)
	}
	physicalMeasurement.Spec.Ip = "192.168.1.11"
	if res := reconciler.reconcileTargetFile(t.Context(), physicalMeasurement); res.Action == steps.ActionError {
		t.Fatalf("second reconcileTargetFile errored: %v", res.Err)
	}

	targetsDir := filepath.Join(tmpDir, prometheusTargetsDir)
	targets := readFileSDTargets(t, filepath.Join(targetsDir, "physical-measurement.json"))
	if len(targets) != 1 {
		t.Fatalf("expected 1 target group, got %d", len(targets))
	}
	if want := []string{"192.168.1.11"}; !slices.Equal(targets[0].Targets, want) {
		t.Errorf("expected targets %v, got %v", want, targets[0].Targets)
	}
	// The atomic write must not leave its temporary file behind.
	assertTargetFiles(t, targetsDir, []string{"physical-measurement.json"})
}

func TestReconcileTargetFile_MultipleMeasurements(t *testing.T) {
	testCases := map[string]struct {
		physicalMeasurements []*chantico.PhysicalMeasurement
		expectedModules      map[string]string // file name -> expected __param_module label
	}{
		"two measurements for different devices": {
			physicalMeasurements: []*chantico.PhysicalMeasurement{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "measurement-1", UID: types.UID("uid-1")},
					Spec: chantico.PhysicalMeasurementSpec{
						MeasurementDevice: "device-type-a",
						Ip:                "192.168.1.10",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "measurement-2", UID: types.UID("uid-2")},
					Spec: chantico.PhysicalMeasurementSpec{
						MeasurementDevice: "device-type-b",
						Ip:                "192.168.1.20",
					},
				},
			},
			expectedModules: map[string]string{
				"measurement-1.json": "device-type-a",
				"measurement-2.json": "device-type-b",
			},
		},
		"two measurements for same device": {
			physicalMeasurements: []*chantico.PhysicalMeasurement{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "measurement-1", UID: types.UID("uid-1")},
					Spec: chantico.PhysicalMeasurementSpec{
						MeasurementDevice: "same-device",
						Ip:                "192.168.1.10",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "measurement-2", UID: types.UID("uid-2")},
					Spec: chantico.PhysicalMeasurementSpec{
						MeasurementDevice: "same-device",
						Ip:                "192.168.1.20",
					},
				},
			},
			expectedModules: map[string]string{
				"measurement-1.json": "same-device",
				"measurement-2.json": "same-device",
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			tmpDir, reconciler := setupPhysicalMeasurementTest(t)

			for _, physicalMeasurement := range tc.physicalMeasurements {
				if res := reconciler.reconcileTargetFile(t.Context(), physicalMeasurement); res.Action == steps.ActionError {
					t.Fatalf("reconcileTargetFile(%s) errored: %v", physicalMeasurement.Name, res.Err)
				}
			}

			targetsDir := filepath.Join(tmpDir, prometheusTargetsDir)
			assertTargetFiles(t, targetsDir, slices.Sorted(maps.Keys(tc.expectedModules)))

			for fileName, expectedModule := range tc.expectedModules {
				targets := readFileSDTargets(t, filepath.Join(targetsDir, fileName))
				if len(targets) != 1 {
					t.Errorf("expected 1 target group in %s, got %d", fileName, len(targets))
					continue
				}
				if got := targets[0].Labels["__param_module"]; got != expectedModule {
					t.Errorf("in %s: expected module %q, got %q", fileName, expectedModule, got)
				}
			}
		})
	}
}

func TestReconcilePhysicalMeasurementDeletion_RemovesTargetFile(t *testing.T) {
	tmpDir, reconciler := setupPhysicalMeasurementTest(t)

	targetsDir := filepath.Join(tmpDir, prometheusTargetsDir)
	targetPath := filepath.Join(targetsDir, "physical-measurement.json")
	if err := os.WriteFile(targetPath, []byte("[]"), 0644); err != nil {
		t.Fatalf("write %s: %v", targetPath, err)
	}

	now := metav1.Now()
	physicalMeasurement := &chantico.PhysicalMeasurement{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "physical-measurement",
			UID:               types.UID("18ac6360-39e7-4ee3-a9b8-58992958e29a"),
			DeletionTimestamp: &now,
			Finalizers:        []string{chantico.PhysicalMeasurementFinalizer},
		},
	}

	res := reconciler.reconcileDeletion(t.Context(), physicalMeasurement)
	if res.Action == steps.ActionError {
		t.Fatalf("reconcileDeletion errored: %v", res.Err)
	}
	if res.Action != steps.ActionStop {
		t.Fatalf("expected Stop, got %v", res.Action)
	}
	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		t.Fatalf("expected target file to be deleted, stat err = %v", err)
	}
	assertTargetFiles(t, targetsDir, nil)
	if controllerutil.ContainsFinalizer(physicalMeasurement, chantico.PhysicalMeasurementFinalizer) {
		t.Fatalf("expected finalizer %q to be removed", chantico.PhysicalMeasurementFinalizer)
	}
}

func TestReconcilePhysicalMeasurementDeletion_NonExistentTargetFile(t *testing.T) {
	_, reconciler := setupPhysicalMeasurementTest(t)

	now := metav1.Now()
	physicalMeasurement := &chantico.PhysicalMeasurement{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "nonexistent",
			DeletionTimestamp: &now,
			Finalizers:        []string{chantico.PhysicalMeasurementFinalizer},
		},
	}

	res := reconciler.reconcileDeletion(t.Context(), physicalMeasurement)
	if res.Action == steps.ActionError {
		t.Fatalf("reconcileDeletion errored: %v", res.Err)
	}
	if res.Action != steps.ActionStop {
		t.Fatalf("expected Stop, got %v", res.Action)
	}
	if controllerutil.ContainsFinalizer(physicalMeasurement, chantico.PhysicalMeasurementFinalizer) {
		t.Fatalf("expected finalizer %q to be removed", chantico.PhysicalMeasurementFinalizer)
	}
}

func TestReconcilePhysicalMeasurementDeletion_NotBeingDeleted(t *testing.T) {
	tmpDir, reconciler := setupPhysicalMeasurementTest(t)

	targetsDir := filepath.Join(tmpDir, prometheusTargetsDir)
	targetPath := filepath.Join(targetsDir, "physical-measurement.json")
	if err := os.WriteFile(targetPath, []byte("[]"), 0644); err != nil {
		t.Fatalf("write %s: %v", targetPath, err)
	}

	physicalMeasurement := &chantico.PhysicalMeasurement{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "physical-measurement",
			Finalizers: []string{chantico.PhysicalMeasurementFinalizer},
		},
	}

	res := reconciler.reconcileDeletion(t.Context(), physicalMeasurement)
	if res.Action != steps.ActionContinue {
		t.Fatalf("expected Continue, got %v", res.Action)
	}
	assertTargetFiles(t, targetsDir, []string{"physical-measurement.json"})
}

func TestEnsurePhysicalMeasurementFinalizerIsSet(t *testing.T) {
	_, reconciler := setupPhysicalMeasurementTest(t)

	physicalMeasurement := &chantico.PhysicalMeasurement{
		ObjectMeta: metav1.ObjectMeta{Name: "physical-measurement"},
	}

	// First run adds the finalizer and stops so the patcher can persist it.
	res := reconciler.ensureFinalizerIsSet(t.Context(), physicalMeasurement)
	if res.Action != steps.ActionStop {
		t.Fatalf("expected Stop, got %v", res.Action)
	}
	if !controllerutil.ContainsFinalizer(physicalMeasurement, chantico.PhysicalMeasurementFinalizer) {
		t.Fatalf("expected finalizer %q to be added", chantico.PhysicalMeasurementFinalizer)
	}

	// Second run is a no-op.
	res = reconciler.ensureFinalizerIsSet(t.Context(), physicalMeasurement)
	if res.Action != steps.ActionContinue {
		t.Fatalf("expected Continue, got %v", res.Action)
	}
}

func TestReconcilePhysicalMeasurementReady(t *testing.T) {
	_, reconciler := setupPhysicalMeasurementTest(t)

	physicalMeasurement := &chantico.PhysicalMeasurement{
		ObjectMeta: metav1.ObjectMeta{Name: "physical-measurement"},
	}

	res := reconciler.reconcileReady(t.Context(), physicalMeasurement)
	if res.Action != steps.ActionContinue {
		t.Fatalf("expected Continue, got %v", res.Action)
	}
	assertPhysicalMeasurementCondition(t, physicalMeasurement, chantico.ConditionReady, metav1.ConditionTrue, chantico.ReasonReconciled)
}

func TestReconcilePhysicalMeasurementValidation_ExistingDevice(t *testing.T) {
	measurementDevice := &chantico.MeasurementDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "device-a", Namespace: "chantico"},
	}
	_, reconciler := setupPhysicalMeasurementTest(t, measurementDevice)

	physicalMeasurement := &chantico.PhysicalMeasurement{
		ObjectMeta: metav1.ObjectMeta{Name: "physical-measurement", Namespace: "chantico"},
		Spec: chantico.PhysicalMeasurementSpec{
			MeasurementDevice: "device-a",
			Ip:                "192.168.1.10",
		},
	}

	res := reconciler.reconcileValidation(t.Context(), physicalMeasurement)
	if res.Action != steps.ActionContinue {
		t.Fatalf("expected Continue, got %v (err: %v)", res.Action, res.Err)
	}
	assertPhysicalMeasurementCondition(t, physicalMeasurement, chantico.ConditionValidated, metav1.ConditionTrue, chantico.ReasonReconciled)
}

func TestReconcilePhysicalMeasurementValidation_MissingDevice(t *testing.T) {
	_, reconciler := setupPhysicalMeasurementTest(t)

	physicalMeasurement := &chantico.PhysicalMeasurement{
		ObjectMeta: metav1.ObjectMeta{Name: "physical-measurement", Namespace: "chantico"},
		Spec: chantico.PhysicalMeasurementSpec{
			MeasurementDevice: "does-not-exist",
			Ip:                "192.168.1.10",
		},
	}

	res := reconciler.reconcileValidation(t.Context(), physicalMeasurement)
	if res.Action != steps.ActionRequeue {
		t.Fatalf("expected Requeue, got %v (err: %v)", res.Action, res.Err)
	}
	if res.RequeueAfter != chantico.EndpointRequeueDelay {
		t.Errorf("expected requeue after %v, got %v", chantico.EndpointRequeueDelay, res.RequeueAfter)
	}
	assertPhysicalMeasurementCondition(t, physicalMeasurement, chantico.ConditionValidated, metav1.ConditionFalse, chantico.ReasonDependencyUnavailable)
}

func TestReconcilePhysicalMeasurementValidation_DeviceInOtherNamespace(t *testing.T) {
	measurementDevice := &chantico.MeasurementDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "device-a", Namespace: "other"},
	}
	_, reconciler := setupPhysicalMeasurementTest(t, measurementDevice)

	physicalMeasurement := &chantico.PhysicalMeasurement{
		ObjectMeta: metav1.ObjectMeta{Name: "physical-measurement", Namespace: "chantico"},
		Spec: chantico.PhysicalMeasurementSpec{
			MeasurementDevice: "device-a",
			Ip:                "192.168.1.10",
		},
	}

	res := reconciler.reconcileValidation(t.Context(), physicalMeasurement)
	if res.Action != steps.ActionRequeue {
		t.Fatalf("expected Requeue, got %v (err: %v)", res.Action, res.Err)
	}
	assertPhysicalMeasurementCondition(t, physicalMeasurement, chantico.ConditionValidated, metav1.ConditionFalse, chantico.ReasonDependencyUnavailable)
}

func assertPhysicalMeasurementCondition(t *testing.T, physicalMeasurement *chantico.PhysicalMeasurement, conditionType chantico.ConditionType, status metav1.ConditionStatus, reason chantico.ConditionReason) {
	t.Helper()

	condition := meta.FindStatusCondition(physicalMeasurement.Status.Conditions, string(conditionType))
	if condition == nil {
		t.Fatalf("expected condition %q to be set", conditionType)
	}
	if condition.Status != status {
		t.Errorf("expected condition %q status %q, got %q", conditionType, status, condition.Status)
	}
	if condition.Reason != string(reason) {
		t.Errorf("expected condition %q reason %q, got %q", conditionType, reason, condition.Reason)
	}
}

func assertTargetFiles(t *testing.T, targetsDir string, want []string) {
	t.Helper()

	entries, err := os.ReadDir(targetsDir)
	if err != nil {
		t.Fatalf("read targets directory %s: %v", targetsDir, err)
	}
	got := []string{}
	for _, entry := range entries {
		if !entry.IsDir() {
			got = append(got, entry.Name())
		}
	}
	if !slices.Equal(got, want) {
		t.Fatalf("expected target files %v, got %v", want, got)
	}
}

func readFileSDTargets(t *testing.T, path string) []pm.FileSDTarget {
	t.Helper()

	targets, err := pm.LoadFileSDTargets(filestore.VolumeFileStore{}, path)
	if err != nil {
		t.Fatalf("read target file %s: %v", path, err)
	}
	return targets
}
