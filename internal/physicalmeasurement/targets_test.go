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

package physicalmeasurement

import (
	"reflect"
	"testing"

	chantico "chantico/api/v1alpha1"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	monitoringv1alpha1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func setupKubePrometheusTargetsTest(t *testing.T) (*KubePrometheusTargets, *chantico.PhysicalMeasurement) {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := monitoringv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	kubeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	targets := &KubePrometheusTargets{Client: kubeClient}
	measurement := &chantico.PhysicalMeasurement{
		ObjectMeta: metav1.ObjectMeta{Name: "measurement", Namespace: "chantico"},
		Spec: chantico.PhysicalMeasurementSpec{
			Type: chantico.PhysicalMeasurementTypePrometheusExporter,
			Ip:   "192.0.2.1:9100",
		},
	}
	return targets, measurement
}

func reconcileKubePrometheusTargetsForTest(t *testing.T, targets *KubePrometheusTargets, measurement *chantico.PhysicalMeasurement) *monitoringv1alpha1.ScrapeConfig {
	t.Helper()

	if err := targets.Reconcile(t.Context(), measurement); err != nil {
		t.Fatal(err)
	}
	observed := &monitoringv1alpha1.ScrapeConfig{}
	if err := targets.Client.Get(t.Context(), client.ObjectKeyFromObject(measurement), observed); err != nil {
		t.Fatal(err)
	}
	return observed
}

func TestKubePrometheusTargetsReconcile_CreatesScrapeConfig(t *testing.T) {
	targets, measurement := setupKubePrometheusTargetsTest(t)
	observed := reconcileKubePrometheusTargetsForTest(t, targets, measurement)

	if got := observed.Spec.StaticConfigs[0].Targets[0]; got != monitoringv1alpha1.Target(measurement.Spec.Ip) {
		t.Fatalf("target = %q, want %q", got, measurement.Spec.Ip)
	}
	if observed.Labels["prometheus"] != "system-monitoring-prometheus" {
		t.Fatalf("unexpected selector labels: %v", observed.Labels)
	}
	if observed.Labels["app.kubernetes.io/managed-by"] != "chantico" {
		t.Fatalf("unexpected managed-by label: %v", observed.Labels)
	}
	if observed.Spec.MetricsPath == nil || *observed.Spec.MetricsPath != "/metrics" {
		t.Fatalf("expected direct exporter metrics path, got %v", observed.Spec.MetricsPath)
	}
	if len(observed.Spec.RelabelConfigs) != 0 {
		t.Fatal("direct exporter has SNMP relabelings")
	}
}

func TestKubePrometheusTargetsReconcile_UpdatesTarget(t *testing.T) {
	targets, measurement := setupKubePrometheusTargetsTest(t)
	reconcileKubePrometheusTargetsForTest(t, targets, measurement)

	measurement.Spec.Ip = "192.0.2.2:9100"
	observed := reconcileKubePrometheusTargetsForTest(t, targets, measurement)

	if got := observed.Spec.StaticConfigs[0].Targets[0]; got != monitoringv1alpha1.Target(measurement.Spec.Ip) {
		t.Fatalf("target = %q, want %q", got, measurement.Spec.Ip)
	}
}

func TestKubePrometheusTargetsReconcile_SkipsUnchangedConfig(t *testing.T) {
	targets, measurement := setupKubePrometheusTargetsTest(t)
	observed := reconcileKubePrometheusTargetsForTest(t, targets, measurement)
	unchanged := reconcileKubePrometheusTargetsForTest(t, targets, measurement)

	if unchanged.ResourceVersion != observed.ResourceVersion {
		t.Fatal("unchanged reconciliation updated the scrape config")
	}
}

func TestKubePrometheusTargetsReconcile_PreservesUnrelatedLabels(t *testing.T) {
	targets, measurement := setupKubePrometheusTargetsTest(t)
	observed := reconcileKubePrometheusTargetsForTest(t, targets, measurement)
	observed.Labels["other"] = "preserved"
	if err := targets.Client.Update(t.Context(), observed); err != nil {
		t.Fatal(err)
	}

	measurement.Spec.Ip = "192.0.2.2:9100"
	updated := reconcileKubePrometheusTargetsForTest(t, targets, measurement)
	if updated.Labels["other"] != "preserved" {
		t.Fatal("reconciliation removed an unrelated label")
	}
}

func TestKubePrometheusTargetsReconcile_RequiresSNMPExporterAddress(t *testing.T) {
	targets, measurement := setupKubePrometheusTargetsTest(t)
	measurement.Spec.Type = chantico.PhysicalMeasurementTypeSNMP

	if err := targets.Reconcile(t.Context(), measurement); err == nil {
		t.Fatal("expected an error for a missing SNMP exporter address")
	}
}

func TestKubePrometheusTargetsReconcile_ConfiguresSNMPExporter(t *testing.T) {
	targets, measurement := setupKubePrometheusTargetsTest(t)
	measurement.Spec.Type = chantico.PhysicalMeasurementTypeSNMP
	measurement.Spec.Ip = "192.0.2.3:1161"
	measurement.Spec.MeasurementDevice = "tno"
	targets.SNMPExporterAddress = "custom-snmp:9116"
	observed := reconcileKubePrometheusTargetsForTest(t, targets, measurement)

	wantRelabelings := []monitoringv1.RelabelConfig{
		{SourceLabels: []monitoringv1.LabelName{"__address__"}, TargetLabel: "__param_target"},
		{SourceLabels: []monitoringv1.LabelName{"__param_target"}, TargetLabel: "instance"},
		{TargetLabel: "__address__", Replacement: &targets.SNMPExporterAddress},
	}
	if !reflect.DeepEqual(observed.Spec.RelabelConfigs, wantRelabelings) {
		t.Fatalf("relabelings = %#v, want %#v", observed.Spec.RelabelConfigs, wantRelabelings)
	}
	if observed.Spec.MetricsPath == nil || *observed.Spec.MetricsPath != "/snmp" {
		t.Fatalf("expected SNMP metrics path, got %v", observed.Spec.MetricsPath)
	}
	if got := observed.Spec.StaticConfigs[0].Targets[0]; got != monitoringv1alpha1.Target(measurement.Spec.Ip) {
		t.Fatalf("SNMP target = %q, want device address %q", got, measurement.Spec.Ip)
	}
	labels := observed.Spec.StaticConfigs[0].Labels
	if labels["__param_auth"] != "tno" || labels["__param_module"] != "tno" {
		t.Fatalf("unexpected SNMP parameters: %v", labels)
	}
}

func TestKubePrometheusTargetsReconcile_RemovesSNMPRelabelingsForDirectExporter(t *testing.T) {
	targets, measurement := setupKubePrometheusTargetsTest(t)
	measurement.Spec.Type = chantico.PhysicalMeasurementTypeSNMP
	targets.SNMPExporterAddress = "custom-snmp:9116"
	reconcileKubePrometheusTargetsForTest(t, targets, measurement)

	measurement.Spec.Type = chantico.PhysicalMeasurementTypePrometheusExporter
	observed := reconcileKubePrometheusTargetsForTest(t, targets, measurement)

	if len(observed.Spec.RelabelConfigs) != 0 {
		t.Fatal("direct exporter retained SNMP relabelings")
	}
	if observed.Spec.MetricsPath == nil || *observed.Spec.MetricsPath != "/metrics" {
		t.Fatalf("expected direct exporter metrics path, got %v", observed.Spec.MetricsPath)
	}
}
