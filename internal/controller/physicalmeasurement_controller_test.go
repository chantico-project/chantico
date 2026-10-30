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
	"context"
	"errors"
	"reflect"
	"testing"

	chantico "chantico/api/v1alpha1"
	"chantico/internal/steps"
	"chantico/internal/testutil"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type reconciledPhysicalMeasurement struct {
	reconcileCalls int
	deleteCalls    int
	deleted        bool
	lastReconciled *chantico.PhysicalMeasurement
	lastDeleted    *chantico.PhysicalMeasurement
}

type TargetConfigurerMock struct {
	reconciledPhysicalMeasurements map[types.NamespacedName]*reconciledPhysicalMeasurement
	reconcileError                 error
	deleteError                    error
}

func (m *TargetConfigurerMock) Reconcile(ctx context.Context, pm *chantico.PhysicalMeasurement) error {
	state := m.measurementState(pm)
	state.reconcileCalls++
	state.deleted = false
	state.lastReconciled = pm.DeepCopy()
	return m.reconcileError
}

func (m *TargetConfigurerMock) Delete(ctx context.Context, pm *chantico.PhysicalMeasurement) error {
	state := m.measurementState(pm)
	state.deleteCalls++
	state.lastDeleted = pm.DeepCopy()
	if m.deleteError != nil {
		return m.deleteError
	}
	state.deleted = true
	return nil
}

func (m *TargetConfigurerMock) measurementState(pm *chantico.PhysicalMeasurement) *reconciledPhysicalMeasurement {
	if m.reconciledPhysicalMeasurements == nil {
		m.reconciledPhysicalMeasurements = make(map[types.NamespacedName]*reconciledPhysicalMeasurement)
	}
	key := types.NamespacedName{Namespace: pm.Namespace, Name: pm.Name}
	if m.reconciledPhysicalMeasurements[key] == nil {
		m.reconciledPhysicalMeasurements[key] = &reconciledPhysicalMeasurement{}
	}
	return m.reconciledPhysicalMeasurements[key]
}

func setupPhysicalMeasurementTest(t *testing.T, objs ...runtime.Object) (*TargetConfigurerMock, *PhysicalMeasurementReconciler) {
	t.Helper()

	client := testutil.NewFakeClientBuilder(t, chantico.AddToScheme).
		WithRuntimeObjects(objs...).
		Build()

	mockConfigurer := &TargetConfigurerMock{}

	return mockConfigurer, &PhysicalMeasurementReconciler{Client: client, Scheme: client.Scheme(), Targets: mockConfigurer}
}

func TestReconcileTargets_DelegatesMeasurement(t *testing.T) {
	mockConfigurer, reconciler := setupPhysicalMeasurementTest(t)

	physicalMeasurement := &chantico.PhysicalMeasurement{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "physical-measurement",
			Namespace: "chantico",
		},
		Spec: chantico.PhysicalMeasurementSpec{
			MeasurementDevice: "device-a",
			Ip:                "192.168.1.10",
			Type:              chantico.PhysicalMeasurementTypeSNMP,
		},
	}

	for i, ip := range []string{"192.168.1.10", "192.168.1.11"} {
		physicalMeasurement.Spec.Ip = ip
		expectedMeasurement := physicalMeasurement.DeepCopy()

		res := reconciler.reconcileTargets(t.Context(), physicalMeasurement)
		if res.Action != steps.ActionContinue || res.Err != nil {
			t.Fatalf("expected Continue without error, got %v (err: %v)", res.Action, res.Err)
		}

		key := types.NamespacedName{Namespace: physicalMeasurement.Namespace, Name: physicalMeasurement.Name}
		state := mockConfigurer.reconciledPhysicalMeasurements[key]
		if state == nil {
			t.Fatal("expected physical measurement to be reconciled")
		}
		if state.reconcileCalls != i+1 {
			t.Errorf("expected %d reconcile calls, got %d", i+1, state.reconcileCalls)
		}
		if !reflect.DeepEqual(state.lastReconciled, expectedMeasurement) {
			t.Errorf("expected reconciled measurement %+v, got %+v", expectedMeasurement, state.lastReconciled)
		}
		if state.deleteCalls != 0 {
			t.Fatal("expected no target deletion")
		}
		assertPhysicalMeasurementCondition(t, physicalMeasurement, chantico.ConditionApplied, metav1.ConditionTrue, chantico.ReasonReconciled)
	}
}

// A configurer reconciliation failure must store the error on the physical measurement
// and mark the target as not applied.
func TestReconcileTargets_ConfigurerError(t *testing.T) {
	mockConfigurer, reconciler := setupPhysicalMeasurementTest(t)
	expectedError := errors.New("target reconciliation failed")
	mockConfigurer.reconcileError = expectedError
	physicalMeasurement := &chantico.PhysicalMeasurement{
		ObjectMeta: metav1.ObjectMeta{Name: "physical-measurement", Namespace: "chantico"},
		Spec:       chantico.PhysicalMeasurementSpec{Type: chantico.PhysicalMeasurementTypeSNMP},
	}
	expectedMeasurement := physicalMeasurement.DeepCopy()

	res := reconciler.reconcileTargets(t.Context(), physicalMeasurement)
	if res.Action != steps.ActionError || res.Err != expectedError {
		t.Fatalf("expected Error with original error, got %v (err: %v)", res.Action, res.Err)
	}
	state := mockConfigurer.reconciledPhysicalMeasurements[types.NamespacedName{Namespace: physicalMeasurement.Namespace, Name: physicalMeasurement.Name}]
	if state == nil {
		t.Fatal("expected target reconciliation to be recorded")
	}
	if state.reconcileCalls != 1 || state.deleteCalls != 0 {
		t.Fatalf("expected one reconcile and no delete calls, got %+v", state)
	}
	if !reflect.DeepEqual(state.lastReconciled, expectedMeasurement) {
		t.Errorf("expected reconciled measurement %+v, got %+v", expectedMeasurement, state.lastReconciled)
	}
	assertPhysicalMeasurementCondition(t, physicalMeasurement, chantico.ConditionApplied, metav1.ConditionFalse, chantico.ReasonApplyFailed)
}

func TestReconcileTargets_SameNameInDifferentNamespaces(t *testing.T) {
	mockConfigurer, reconciler := setupPhysicalMeasurementTest(t)
	for _, namespace := range []string{"chantico", "other"} {
		measurement := &chantico.PhysicalMeasurement{
			ObjectMeta: metav1.ObjectMeta{Name: "measurement", Namespace: namespace},
			Spec:       chantico.PhysicalMeasurementSpec{Type: chantico.PhysicalMeasurementTypeSNMP},
		}
		if res := reconciler.reconcileTargets(t.Context(), measurement); res.Action != steps.ActionContinue {
			t.Fatalf("expected Continue, got %v (err: %v)", res.Action, res.Err)
		}
	}

	if len(mockConfigurer.reconciledPhysicalMeasurements) != 2 {
		t.Fatalf("expected 2 tracked measurements, got %d", len(mockConfigurer.reconciledPhysicalMeasurements))
	}
	for _, namespace := range []string{"chantico", "other"} {
		state := mockConfigurer.reconciledPhysicalMeasurements[types.NamespacedName{Namespace: namespace, Name: "measurement"}]
		if state == nil || state.lastReconciled == nil {
			t.Fatalf("expected a recorded measurement in namespace %q", namespace)
		}
		if state.reconcileCalls != 1 || state.lastReconciled.Namespace != namespace {
			t.Errorf("expected one reconcile in namespace %q, got %+v", namespace, state)
		}
	}
}

func TestReconcilePhysicalMeasurementValidation_Type(t *testing.T) {
	testCases := map[string]struct {
		measurementType chantico.PhysicalMeasurementType
		expectedError   string
	}{
		"snmp": {
			measurementType: chantico.PhysicalMeasurementTypeSNMP,
		},
		"prometheus exporter": {
			measurementType: chantico.PhysicalMeasurementTypePrometheusExporter,
		},
		"unsupported type": {
			measurementType: "unsupported",
			expectedError:   `unsupported physical measurement type: "unsupported"`,
		},
		// This won't happen in real operation due to defaulting to SNMP in the CRD definiton, but worth checking that it rejects an empty value
		"empty type": {
			expectedError: `unsupported physical measurement type: ""`,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			measurementDevice := &chantico.MeasurementDevice{
				ObjectMeta: metav1.ObjectMeta{Name: "device-a", Namespace: "chantico"},
			}
			mockConfigurer, reconciler := setupPhysicalMeasurementTest(t, measurementDevice)
			physicalMeasurement := &chantico.PhysicalMeasurement{
				ObjectMeta: metav1.ObjectMeta{Name: "physical-measurement", Namespace: "chantico"},
				Spec: chantico.PhysicalMeasurementSpec{
					Type:              tc.measurementType,
					MeasurementDevice: "device-a",
				},
			}
			if tc.measurementType == chantico.PhysicalMeasurementTypePrometheusExporter {
				physicalMeasurement.Spec.MeasurementDevice = ""
			}

			res := reconciler.reconcileValidation(t.Context(), physicalMeasurement)
			if tc.expectedError == "" {
				if res.Action != steps.ActionContinue || res.Err != nil {
					t.Fatalf("expected Continue without error, got %v (err: %v)", res.Action, res.Err)
				}
				assertPhysicalMeasurementCondition(t, physicalMeasurement, chantico.ConditionValidated, metav1.ConditionTrue, chantico.ReasonReconciled)
				return
			}

			if res.Action != steps.ActionError {
				t.Fatalf("expected Error, got %v", res.Action)
			}
			if res.Err == nil || res.Err.Error() != tc.expectedError {
				t.Fatalf("expected error %q, got %v", tc.expectedError, res.Err)
			}
			assertPhysicalMeasurementCondition(t, physicalMeasurement, chantico.ConditionValidated, metav1.ConditionFalse, chantico.ReasonInvalidSpec)

			if _, err := steps.Run(t.Context(), physicalMeasurement, reconciler.reconcileValidation, reconciler.reconcileTargets); err == nil {
				t.Fatal("expected validation to stop reconciliation with an error")
			}

			if len(mockConfigurer.reconciledPhysicalMeasurements) != 0 {
				t.Fatal("expected no target configurer calls")
			}
			if meta.FindStatusCondition(physicalMeasurement.Status.Conditions, string(chantico.ConditionApplied)) != nil {
				t.Fatal("validation failure should not set the Applied condition")
			}
		})
	}
}

func TestReconcilePhysicalMeasurementDeletion_RemovesTarget(t *testing.T) {
	mockConfigurer, reconciler := setupPhysicalMeasurementTest(t)

	now := metav1.Now()
	physicalMeasurement := &chantico.PhysicalMeasurement{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "physical-measurement",
			UID:               types.UID("18ac6360-39e7-4ee3-a9b8-58992958e29a"),
			DeletionTimestamp: &now,
			Finalizers:        []string{chantico.PhysicalMeasurementFinalizer},
		},
		Spec: chantico.PhysicalMeasurementSpec{Type: chantico.PhysicalMeasurementTypeSNMP},
	}

	expectedMeasurement := physicalMeasurement.DeepCopy()

	res := reconciler.reconcileDeletion(t.Context(), physicalMeasurement)
	if res.Action == steps.ActionError {
		t.Fatalf("reconcileDeletion errored: %v", res.Err)
	}
	if res.Action != steps.ActionStop {
		t.Fatalf("expected Stop, got %v", res.Action)
	}
	state := mockConfigurer.reconciledPhysicalMeasurements[types.NamespacedName{Name: physicalMeasurement.Name}]
	if state == nil {
		t.Fatal("expected physical measurement deletion to be recorded")
	}
	if state.deleteCalls != 1 || !state.deleted {
		t.Fatalf("expected one deletion, got %d calls (deleted: %v)", state.deleteCalls, state.deleted)
	}
	if state.reconcileCalls != 0 {
		t.Errorf("expected no reconcile calls, got %d", state.reconcileCalls)
	}
	if !reflect.DeepEqual(state.lastDeleted, expectedMeasurement) {
		t.Errorf("expected deleted measurement %+v, got %+v", expectedMeasurement, state.lastDeleted)
	}
	if controllerutil.ContainsFinalizer(physicalMeasurement, chantico.PhysicalMeasurementFinalizer) {
		t.Fatalf("expected finalizer %q to be removed", chantico.PhysicalMeasurementFinalizer)
	}
}

// Failed cleanup must result in an error state for the physical measurement and the finalizer must remain so deletion can be retired.
func TestReconcilePhysicalMeasurementDeletion_ConfigurerError(t *testing.T) {
	mockConfigurer, reconciler := setupPhysicalMeasurementTest(t)
	expectedError := errors.New("target deletion failed")
	mockConfigurer.deleteError = expectedError
	now := metav1.Now()
	physicalMeasurement := &chantico.PhysicalMeasurement{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "physical-measurement",
			Namespace:         "chantico",
			DeletionTimestamp: &now,
			Finalizers:        []string{chantico.PhysicalMeasurementFinalizer},
		},
	}
	expectedMeasurement := physicalMeasurement.DeepCopy()

	res := reconciler.reconcileDeletion(t.Context(), physicalMeasurement)
	if res.Action != steps.ActionError || res.Err != expectedError {
		t.Fatalf("expected Error with original error, got %v (err: %v)", res.Action, res.Err)
	}
	state := mockConfigurer.reconciledPhysicalMeasurements[types.NamespacedName{Namespace: physicalMeasurement.Namespace, Name: physicalMeasurement.Name}]
	if state == nil {
		t.Fatal("expected target deletion attempt to be recorded")
	}
	if state.deleteCalls != 1 || state.reconcileCalls != 0 || state.deleted {
		t.Fatalf("expected one failed delete and no reconcile calls, got %+v", state)
	}
	if !reflect.DeepEqual(state.lastDeleted, expectedMeasurement) {
		t.Errorf("expected deleted measurement %+v, got %+v", expectedMeasurement, state.lastDeleted)
	}
	assertPhysicalMeasurementCondition(t, physicalMeasurement, chantico.ConditionApplied, metav1.ConditionFalse, chantico.ReasonCleanupFailed)
	if !controllerutil.ContainsFinalizer(physicalMeasurement, chantico.PhysicalMeasurementFinalizer) {
		t.Fatal("expected finalizer to be retained for cleanup retry")
	}
}

func TestReconcilePhysicalMeasurementDeletion_NotBeingDeleted(t *testing.T) {
	mockConfigurer, reconciler := setupPhysicalMeasurementTest(t)
	physicalMeasurement := &chantico.PhysicalMeasurement{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "physical-measurement",
			Finalizers: []string{chantico.PhysicalMeasurementFinalizer},
		},
	}
	expectedMeasurement := physicalMeasurement.DeepCopy()

	res := reconciler.reconcileDeletion(t.Context(), physicalMeasurement)
	if res.Action != steps.ActionContinue || res.Err != nil {
		t.Fatalf("expected Continue without error, got %v (err: %v)", res.Action, res.Err)
	}
	if len(mockConfigurer.reconciledPhysicalMeasurements) != 0 {
		t.Fatal("expected no target configurer calls")
	}
	if !reflect.DeepEqual(physicalMeasurement, expectedMeasurement) {
		t.Errorf("expected measurement to remain unchanged, got %+v", physicalMeasurement)
	}
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
			Type:              chantico.PhysicalMeasurementTypeSNMP,
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
			Type:              chantico.PhysicalMeasurementTypeSNMP,
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
			Type:              chantico.PhysicalMeasurementTypeSNMP,
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
