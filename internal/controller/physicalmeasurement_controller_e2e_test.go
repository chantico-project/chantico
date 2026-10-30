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
	"chantico/internal/physicalmeasurement"
	"chantico/internal/testutil"

	monitoringv1alpha1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// This test file runs an end to end test whch exercises a basic physical measurement reconciliation against the k8s API
// with a concrete Kube Prometheus Configurer to check that the files are written to the API.

func setupPhysicalMeasurementIntegrationTest(t *testing.T) (*PhysicalMeasurementReconciler, *chantico.PhysicalMeasurement) {
	t.Helper()

	kubeClient := testutil.NewFakeClientBuilder(t, chantico.AddToScheme, monitoringv1alpha1.AddToScheme).
		WithStatusSubresource(&chantico.PhysicalMeasurement{}).
		Build()
	reconciler := &PhysicalMeasurementReconciler{
		Client:  kubeClient,
		Scheme:  kubeClient.Scheme(),
		Targets: &physicalmeasurement.KubePrometheusTargets{Client: kubeClient},
	}
	measurement := &chantico.PhysicalMeasurement{
		ObjectMeta: metav1.ObjectMeta{Name: "measurement", Namespace: "chantico"},
		Spec: chantico.PhysicalMeasurementSpec{
			Type: chantico.PhysicalMeasurementTypePrometheusExporter,
			Ip:   "192.0.2.1:9100",
		},
	}
	if err := kubeClient.Create(t.Context(), measurement); err != nil {
		t.Fatal(err)
	}

	// The first reconciliation persists the finalizer; the second configures the target.
	for range 2 {
		reconcilePhysicalMeasurementIntegrationTest(t, reconciler, measurement)
	}

	identifier := client.ObjectKey{Namespace: measurement.Namespace, Name: measurement.Name}
	if err := kubeClient.Get(t.Context(), identifier, measurement); err != nil {
		t.Fatal(err)
	}
	if !controllerutil.ContainsFinalizer(measurement, chantico.PhysicalMeasurementFinalizer) {
		t.Fatal("expected the physical measurement finalizer to be persisted")
	}
	assertPhysicalMeasurementCondition(t, measurement, chantico.ConditionReady, metav1.ConditionTrue, chantico.ReasonReconciled)

	return reconciler, measurement
}

func reconcilePhysicalMeasurementIntegrationTest(t *testing.T, reconciler *PhysicalMeasurementReconciler, measurement *chantico.PhysicalMeasurement) {
	t.Helper()

	result, err := reconciler.Reconcile(t.Context(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(measurement)})
	if err != nil {
		t.Fatal(err)
	}
	if result != (ctrl.Result{}) {
		t.Fatalf("unexpected reconciliation result: %+v", result)
	}
}

func TestPhysicalMeasurementIntegration_CreatesScrapeConfig(t *testing.T) {
	reconciler, measurement := setupPhysicalMeasurementIntegrationTest(t)

	scrapeConfig := &monitoringv1alpha1.ScrapeConfig{}
	if err := reconciler.Get(t.Context(), client.ObjectKeyFromObject(measurement), scrapeConfig); err != nil {
		t.Fatalf("get generated scrape config: %v", err)
	}
	if len(scrapeConfig.Spec.StaticConfigs) != 1 || len(scrapeConfig.Spec.StaticConfigs[0].Targets) != 1 {
		t.Fatalf("expected one static target, got %+v", scrapeConfig.Spec.StaticConfigs)
	}
	if got := scrapeConfig.Spec.StaticConfigs[0].Targets[0]; got != monitoringv1alpha1.Target(measurement.Spec.Ip) {
		t.Errorf("scrape target = %q, want %q", got, measurement.Spec.Ip)
	}
	if scrapeConfig.Spec.MetricsPath == nil || *scrapeConfig.Spec.MetricsPath != "/metrics" {
		t.Errorf("expected exporter metrics path /metrics, got %v", scrapeConfig.Spec.MetricsPath)
	}
	assertPhysicalMeasurementCondition(t, measurement, chantico.ConditionApplied, metav1.ConditionTrue, chantico.ReasonReconciled)
}

func TestPhysicalMeasurementIntegration_DeletesScrapeConfig(t *testing.T) {
	reconciler, measurement := setupPhysicalMeasurementIntegrationTest(t)
	key := client.ObjectKeyFromObject(measurement)
	if err := reconciler.Get(t.Context(), key, &monitoringv1alpha1.ScrapeConfig{}); err != nil {
		t.Fatalf("expected a scrape config before deletion: %v", err)
	}

	if err := reconciler.Delete(t.Context(), measurement); err != nil {
		t.Fatal(err)
	}
	if err := reconciler.Get(t.Context(), key, measurement); err != nil {
		t.Fatalf("expected the finalizer to retain the deleting measurement: %v", err)
	}
	if measurement.DeletionTimestamp == nil {
		t.Fatal("expected the measurement to be marked for deletion")
	}

	reconcilePhysicalMeasurementIntegrationTest(t, reconciler, measurement)

	if err := reconciler.Get(t.Context(), key, &monitoringv1alpha1.ScrapeConfig{}); !apierrors.IsNotFound(err) {
		t.Fatalf("expected the scrape config to be deleted, got %v", err)
	}
	if err := reconciler.Get(t.Context(), key, &chantico.PhysicalMeasurement{}); !apierrors.IsNotFound(err) {
		t.Fatalf("expected the measurement to be deleted after finalizer cleanup, got %v", err)
	}
}
