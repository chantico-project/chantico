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
	"testing"

	chantico "chantico/api/v1alpha1"
	"chantico/internal/steps"
	"chantico/internal/testutil"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestReconcileTargets_InitializesOnlyMissingHealthConditions(t *testing.T) {
	_, reconciler := setupPhysicalMeasurementTest(t)
	physicalMeasurement := &chantico.PhysicalMeasurement{
		ObjectMeta: metav1.ObjectMeta{Name: testPM, Namespace: testNamespace},
		Spec:       chantico.PhysicalMeasurementSpec{Ip: testIpA, Type: chantico.PhysicalMeasurementTypePrometheusExporter},
	}
	physicalMeasurement.UpdateStatusCondition(chantico.ConditionEndpointReachable, metav1.ConditionFalse, chantico.ReasonAlertFiring, "down")

	if res := reconciler.reconcileTargets(t.Context(), physicalMeasurement); res.Action != steps.ActionContinue {
		t.Fatalf("expected Continue, got %v (%v)", res.Action, res.Err)
	}

	assertPhysicalMeasurementCondition(t, physicalMeasurement, chantico.ConditionScrapeConfigLoaded, metav1.ConditionTrue, chantico.ReasonReconciled)
	assertPhysicalMeasurementCondition(t, physicalMeasurement, chantico.ConditionEndpointReachable, metav1.ConditionFalse, chantico.ReasonAlertFiring)
}

func TestControllerPatch_KeepsConcurrentHealthCondition(t *testing.T) {
	physicalMeasurement := &chantico.PhysicalMeasurement{
		ObjectMeta: metav1.ObjectMeta{Name: testPM, Namespace: testNamespace},
	}
	physicalMeasurement.UpdateStatusCondition(chantico.ConditionEndpointReachable, metav1.ConditionTrue, chantico.ReasonReconciled, "")
	kubeClient := testutil.NewFakeClientBuilder(t, chantico.AddToScheme).
		WithStatusSubresource(&chantico.PhysicalMeasurement{}).
		WithObjects(physicalMeasurement).
		Build()
	key := client.ObjectKeyFromObject(physicalMeasurement)

	reconciled := &chantico.PhysicalMeasurement{}
	if err := kubeClient.Get(t.Context(), key, reconciled); err != nil {
		t.Fatal(err)
	}
	patcher, err := patch.NewHelper(reconciled, kubeClient)
	if err != nil {
		t.Fatal(err)
	}

	// The alert webhook updates the health condition while the controller reconciles.
	alerted := &chantico.PhysicalMeasurement{}
	if err := kubeClient.Get(t.Context(), key, alerted); err != nil {
		t.Fatal(err)
	}
	alerted.UpdateStatusCondition(chantico.ConditionEndpointReachable, metav1.ConditionFalse, chantico.ReasonAlertFiring, "down")
	if err := kubeClient.Status().Update(t.Context(), alerted); err != nil {
		t.Fatal(err)
	}

	reconciled.UpdateStatusCondition(chantico.ConditionReady, metav1.ConditionTrue, chantico.ReasonReconciled, "")
	if err := patcher.Patch(t.Context(), reconciled, patch.WithOwnedConditions{Conditions: chantico.ControllerConditions}); err != nil {
		t.Fatal(err)
	}

	stored := &chantico.PhysicalMeasurement{}
	if err := kubeClient.Get(t.Context(), key, stored); err != nil {
		t.Fatal(err)
	}
	if !meta.IsStatusConditionFalse(stored.Status.Conditions, string(chantico.ConditionEndpointReachable)) {
		t.Errorf("controller patch overwrote the health condition: %v", stored.Status.Conditions)
	}
	if !meta.IsStatusConditionTrue(stored.Status.Conditions, string(chantico.ConditionReady)) {
		t.Errorf("controller patch did not set Ready: %v", stored.Status.Conditions)
	}
}
