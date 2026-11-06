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
	"fmt"
	"maps"
	"slices"
	"testing"

	chantico "chantico/api/v1alpha1"
	pm "chantico/internal/physicalmeasurement"
	"chantico/internal/steps"
	"chantico/internal/testutil"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	monitoringv1alpha1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1alpha1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	testNamespace                    = "chantico"
	testMeasurementA                 = "measurement-1"
	testMeasurementB                 = "measurement-2"
	testDeviceA                      = "device-type-a"
	testDeviceB                      = "device-type-b"
	testPM                           = "physical-measurement"
	testIpA                          = "192.168.1.10"
	testIpB                          = "192.168.1.20"
	testResourceIdLabel              = "resourceId"
	testResourceId                   = "74366c42-ac0c-4736-8301-db6d3c63297a"
	testModuleTypeSNMP               = "snmp"
	testModuleTypePrometheusExporter = "prometheus-exporter"
	testSNMPExporterAddress          = "chantico-snmp.chantico.svc:9116"
)

func setupPhysicalMeasurementTest(t *testing.T, objs ...runtime.Object) (client.WithWatch, *PhysicalMeasurementReconciler) {
	t.Helper()

	kubeClient := testutil.NewFakeClientBuilder(t, chantico.AddToScheme, monitoringv1.AddToScheme, monitoringv1alpha1.AddToScheme).
		WithStatusSubresource(&chantico.PhysicalMeasurement{}).
		WithRuntimeObjects(objs...).
		Build()

	targetConfigurer := &pm.PrometheusTargetConfigurer{
		Client:              kubeClient,
		SNMPExporterAddress: testSNMPExporterAddress,
	}

	return kubeClient, &PhysicalMeasurementReconciler{Client: kubeClient, TargetConfigurer: targetConfigurer}
}

func TestReconcileTargetFile_WritesTargetFile(t *testing.T) {
	client, reconciler := setupPhysicalMeasurementTest(t)

	physicalMeasurement := &chantico.PhysicalMeasurement{
		ObjectMeta: metav1.ObjectMeta{
			Name: testPM,
			UID:  types.UID("18ac6360-39e7-4ee3-a9b8-58992958e29a"),
		},
		Spec: chantico.PhysicalMeasurementSpec{
			MeasurementDevice: testDeviceA,
			Ip:                testIpA,
			Type:              chantico.PhysicalMeasurementTypeSNMP,
			AdditionalLabels: map[string]string{
				testResourceIdLabel: testResourceId,
			},
		},
	}
	wantLabels := map[string]string{
		"__param_module":    testDeviceA,
		"__param_auth":      testDeviceA,
		"job":               testDeviceA,
		"type":              testModuleTypeSNMP,
		"name":              physicalMeasurement.Name,
		"instance":          physicalMeasurement.Spec.Ip,
		testResourceIdLabel: testResourceId,
	}

	res := reconciler.reconcileTargets(t.Context(), physicalMeasurement)
	if res.Action == steps.ActionError {
		t.Fatalf("reconcileTargetFile errored: %v", res.Err)
	}
	if res.Action != steps.ActionContinue {
		t.Fatalf("expected Continue, got %v", res.Action)
	}

	targetsResource, err := readScrapeConfigResource(t, client, physicalMeasurement)
	if err != nil {
		t.Fatalf("reading scrape config resource: %v", err)
	}
	if len(targetsResource.Spec.StaticConfigs) != 1 {
		t.Fatalf("expected 1 target group, got %d", len(targetsResource.Spec.StaticConfigs))
	}
	if !maps.Equal(targetsResource.Spec.StaticConfigs[0].Labels, wantLabels) {
		t.Errorf("expected labels %v, got %v", wantLabels, targetsResource.Spec.StaticConfigs[0].Labels)
	}
	if !slices.Equal(targetsResource.Spec.StaticConfigs[0].Targets, []monitoringv1alpha1.Target{monitoringv1alpha1.Target(physicalMeasurement.Spec.Ip)}) {
		t.Errorf("expected targets %v, got %v", []monitoringv1alpha1.Target{monitoringv1alpha1.Target(physicalMeasurement.Spec.Ip)}, targetsResource.Spec.StaticConfigs[0].Targets)
	}

	assertPhysicalMeasurementCondition(t, physicalMeasurement, chantico.ConditionApplied, metav1.ConditionTrue, chantico.ReasonReconciled)
}

func TestReconcileTargetFile_SkipsWriteWhenUnchanged(t *testing.T) {
	client, reconciler := setupPhysicalMeasurementTest(t)

	physicalMeasurement := &chantico.PhysicalMeasurement{
		ObjectMeta: metav1.ObjectMeta{Name: testPM},
		Spec: chantico.PhysicalMeasurementSpec{
			MeasurementDevice: testDeviceA,
			Ip:                testIpA,
			Type:              chantico.PhysicalMeasurementTypeSNMP,
		},
	}

	if res := reconciler.reconcileTargets(t.Context(), physicalMeasurement); res.Action == steps.ActionError {
		t.Fatalf("first reconcileTargets errored: %v", res.Err)
	}

	first, err := readScrapeConfigResource(t, client, physicalMeasurement)
	if err != nil {
		t.Fatalf("reading scrape config resource: %v", err)
	}

	if res := reconciler.reconcileTargets(t.Context(), physicalMeasurement); res.Action == steps.ActionError {
		t.Fatalf("second reconcileTargets errored: %v", res.Err)
	}
	second, err := readScrapeConfigResource(t, client, physicalMeasurement)
	if err != nil {
		t.Fatalf("reading scrape config resource: %v", err)
	}

	if first.ResourceVersion != second.ResourceVersion {
		t.Errorf("expected ScrapeConfig not to be rewritten, resourceVersion changed from %q to %q", first.ResourceVersion, second.ResourceVersion)
	}

	assertPhysicalMeasurementCondition(t, physicalMeasurement, chantico.ConditionApplied, metav1.ConditionTrue, chantico.ReasonReconciled)
}

func TestReconcileTargets_UpdatesWhenChanged(t *testing.T) {
	client, reconciler := setupPhysicalMeasurementTest(t)

	physicalMeasurement := &chantico.PhysicalMeasurement{
		ObjectMeta: metav1.ObjectMeta{Name: testPM},
		Spec: chantico.PhysicalMeasurementSpec{
			MeasurementDevice: testDeviceA,
			Ip:                testIpA,
			Type:              chantico.PhysicalMeasurementTypeSNMP,
		},
	}

	if res := reconciler.reconcileTargets(t.Context(), physicalMeasurement); res.Action == steps.ActionError {
		t.Fatalf("first reconcileTargets errored: %v", res.Err)
	}
	first, err := readScrapeConfigResource(t, client, physicalMeasurement)
	if err != nil {
		t.Fatalf("reading scrape config resource: %v", err)
	}

	physicalMeasurement.Spec.Ip = "192.168.1.99"
	if res := reconciler.reconcileTargets(t.Context(), physicalMeasurement); res.Action == steps.ActionError {
		t.Fatalf("second reconcileTargets errored: %v", res.Err)
	}
	second, err := readScrapeConfigResource(t, client, physicalMeasurement)
	if err != nil {
		t.Fatalf("reading scrape config resource: %v", err)
	}

	if first.ResourceVersion == second.ResourceVersion {
		t.Errorf("expected ScrapeConfig to be updated, resourceVersion stayed %q", first.ResourceVersion)
	}
	wantTargets := []monitoringv1alpha1.Target{monitoringv1alpha1.Target(physicalMeasurement.Spec.Ip)}
	if !slices.Equal(second.Spec.StaticConfigs[0].Targets, wantTargets) {
		t.Errorf("expected targets %v, got %v", wantTargets, second.Spec.StaticConfigs[0].Targets)
	}
}

func TestReconcileTargets_MultipleMeasurements(t *testing.T) {
	const testDeviceSame = "same-device"

	type expectedScrapeConfig struct {
		ip         string
		targetType string
		module     string // expected __param_module label; empty for non-SNMP targets
	}

	newMeasurement := func(name, uid, device, ip string, measurementType chantico.PhysicalMeasurementType) *chantico.PhysicalMeasurement {
		return &chantico.PhysicalMeasurement{
			ObjectMeta: metav1.ObjectMeta{Name: name, UID: types.UID(uid)},
			Spec: chantico.PhysicalMeasurementSpec{
				MeasurementDevice: device,
				Ip:                ip,
				Type:              measurementType,
			},
		}
	}

	testCases := map[string]struct {
		physicalMeasurements []*chantico.PhysicalMeasurement
		expected             map[string]expectedScrapeConfig // measurement name -> expected ScrapeConfig contents
	}{
		"two measurements for different devices": {
			physicalMeasurements: []*chantico.PhysicalMeasurement{
				newMeasurement(testMeasurementA, "uid-1", testDeviceA, testIpA, chantico.PhysicalMeasurementTypeSNMP),
				newMeasurement(testMeasurementB, "uid-2", testDeviceB, testIpB, chantico.PhysicalMeasurementTypeSNMP),
			},
			expected: map[string]expectedScrapeConfig{
				testMeasurementA: {ip: testIpA, targetType: testModuleTypeSNMP, module: testDeviceA},
				testMeasurementB: {ip: testIpB, targetType: testModuleTypeSNMP, module: testDeviceB},
			},
		},
		"two measurements for same device": {
			physicalMeasurements: []*chantico.PhysicalMeasurement{
				newMeasurement(testMeasurementA, "uid-1", testDeviceSame, testIpA, chantico.PhysicalMeasurementTypeSNMP),
				newMeasurement(testMeasurementB, "uid-2", testDeviceSame, testIpB, chantico.PhysicalMeasurementTypeSNMP),
			},
			expected: map[string]expectedScrapeConfig{
				testMeasurementA: {ip: testIpA, targetType: testModuleTypeSNMP, module: testDeviceSame},
				testMeasurementB: {ip: testIpB, targetType: testModuleTypeSNMP, module: testDeviceSame},
			},
		},
		"one snmp and one prometheus exporter measurement": {
			physicalMeasurements: []*chantico.PhysicalMeasurement{
				newMeasurement(testMeasurementA, "uid-1", testDeviceA, testIpA, chantico.PhysicalMeasurementTypeSNMP),
				newMeasurement(testMeasurementB, "uid-2", testDeviceB, testIpB, chantico.PhysicalMeasurementTypePrometheusExporter),
			},
			expected: map[string]expectedScrapeConfig{
				testMeasurementA: {ip: testIpA, targetType: testModuleTypeSNMP, module: testDeviceA},
				testMeasurementB: {ip: testIpB, targetType: testModuleTypePrometheusExporter},
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			client, reconciler := setupPhysicalMeasurementTest(t)

			for _, physicalMeasurement := range tc.physicalMeasurements {
				if res := reconciler.reconcileTargets(t.Context(), physicalMeasurement); res.Action == steps.ActionError {
					t.Fatalf("reconcileTargets(%s) errored: %v", physicalMeasurement.Name, res.Err)
				}
			}

			scrapeConfigs := &monitoringv1alpha1.ScrapeConfigList{}
			if err := client.List(t.Context(), scrapeConfigs); err != nil {
				t.Fatalf("listing ScrapeConfigs: %v", err)
			}
			if len(scrapeConfigs.Items) != len(tc.expected) {
				t.Fatalf("expected %d ScrapeConfigs, got %d", len(tc.expected), len(scrapeConfigs.Items))
			}

			for _, physicalMeasurement := range tc.physicalMeasurements {
				want := tc.expected[physicalMeasurement.Name]
				scrapeConfig, err := readScrapeConfigResource(t, client, physicalMeasurement)
				if err != nil {
					t.Fatalf("%s: %v", physicalMeasurement.Name, err)
				}
				if len(scrapeConfig.Spec.StaticConfigs) != 1 {
					t.Errorf("%s: expected 1 static config, got %d", physicalMeasurement.Name, len(scrapeConfig.Spec.StaticConfigs))
					continue
				}
				staticConfig := scrapeConfig.Spec.StaticConfigs[0]

				wantTargets := []monitoringv1alpha1.Target{monitoringv1alpha1.Target(want.ip)}
				if !slices.Equal(staticConfig.Targets, wantTargets) {
					t.Errorf("%s: expected targets %v, got %v", physicalMeasurement.Name, wantTargets, staticConfig.Targets)
				}
				if got := staticConfig.Labels["type"]; got != want.targetType {
					t.Errorf("%s: expected type %q, got %q", physicalMeasurement.Name, want.targetType, got)
				}
				if got := staticConfig.Labels["__param_module"]; got != want.module {
					t.Errorf("%s: expected module %q, got %q", physicalMeasurement.Name, want.module, got)
				}

				// Only SNMP targets are relabeled to scrape through the SNMP exporter.
				isSNMP := want.targetType == testModuleTypeSNMP
				if hasRelabel := len(scrapeConfig.Spec.RelabelConfigs) > 0; hasRelabel != isSNMP {
					t.Errorf("%s: expected relabel configs present=%v, got %v", physicalMeasurement.Name, isSNMP, scrapeConfig.Spec.RelabelConfigs)
				}
			}
		})
	}
}

func TestReconcileTargets_PrometheusExporter(t *testing.T) {
	client, reconciler := setupPhysicalMeasurementTest(t)

	physicalMeasurement := &chantico.PhysicalMeasurement{
		ObjectMeta: metav1.ObjectMeta{Name: "exporter"},
		Spec: chantico.PhysicalMeasurementSpec{
			Ip:   "192.168.1.10:9100",
			Type: chantico.PhysicalMeasurementTypePrometheusExporter,
			AdditionalLabels: map[string]string{
				testResourceIdLabel: testResourceId,
			},
		},
	}
	wantLabels := map[string]string{
		"name":              "exporter",
		"instance":          physicalMeasurement.Spec.Ip,
		"type":              testModuleTypePrometheusExporter,
		testResourceIdLabel: testResourceId,
	}

	if res := reconciler.reconcileTargets(t.Context(), physicalMeasurement); res.Action != steps.ActionContinue {
		t.Fatalf("expected Continue, got %v (err: %v)", res.Action, res.Err)
	}

	scrapeConfig, err := readScrapeConfigResource(t, client, physicalMeasurement)
	if err != nil {
		t.Fatalf("reading scrape config resource: %v", err)
	}
	if len(scrapeConfig.Spec.StaticConfigs) != 1 {
		t.Fatalf("expected 1 target group, got %d", len(scrapeConfig.Spec.StaticConfigs))
	}
	staticConfig := scrapeConfig.Spec.StaticConfigs[0]
	if want := []monitoringv1alpha1.Target{monitoringv1alpha1.Target(physicalMeasurement.Spec.Ip)}; !slices.Equal(staticConfig.Targets, want) {
		t.Errorf("expected targets %v, got %v", want, staticConfig.Targets)
	}
	if !maps.Equal(staticConfig.Labels, wantLabels) {
		t.Errorf("expected labels %v, got %v", wantLabels, staticConfig.Labels)
	}
	if len(scrapeConfig.Spec.RelabelConfigs) != 0 {
		t.Errorf("expected no relabel configs for a prometheus exporter, got %v", scrapeConfig.Spec.RelabelConfigs)
	}
}

func TestReconcileTargets_SNMPRelabelConfigs(t *testing.T) {
	client, reconciler := setupPhysicalMeasurementTest(t)

	physicalMeasurement := &chantico.PhysicalMeasurement{
		ObjectMeta: metav1.ObjectMeta{Name: testPM},
		Spec: chantico.PhysicalMeasurementSpec{
			MeasurementDevice: testDeviceA,
			Ip:                testIpA,
			Type:              chantico.PhysicalMeasurementTypeSNMP,
		},
	}

	if res := reconciler.reconcileTargets(t.Context(), physicalMeasurement); res.Action != steps.ActionContinue {
		t.Fatalf("expected Continue, got %v (err: %v)", res.Action, res.Err)
	}

	scrapeConfig, err := readScrapeConfigResource(t, client, physicalMeasurement)
	if err != nil {
		t.Fatalf("reading scrape config resource: %v", err)
	}
	relabelConfigs := scrapeConfig.Spec.RelabelConfigs
	if len(relabelConfigs) != 3 {
		t.Fatalf("expected 3 relabel configs, got %d: %v", len(relabelConfigs), relabelConfigs)
	}
}

func TestReconcileTargets_SNMPRequiresExporterAddress(t *testing.T) {
	_, reconciler := setupPhysicalMeasurementTest(t)
	reconciler.TargetConfigurer.(*pm.PrometheusTargetConfigurer).SNMPExporterAddress = ""

	physicalMeasurement := &chantico.PhysicalMeasurement{
		ObjectMeta: metav1.ObjectMeta{Name: testPM},
		Spec: chantico.PhysicalMeasurementSpec{
			MeasurementDevice: testDeviceA,
			Ip:                testIpA,
			Type:              chantico.PhysicalMeasurementTypeSNMP,
		},
	}

	res := reconciler.reconcileTargets(t.Context(), physicalMeasurement)
	if res.Action != steps.ActionError {
		t.Fatalf("expected Error, got %v", res.Action)
	}
	assertPhysicalMeasurementCondition(t, physicalMeasurement, chantico.ConditionApplied, metav1.ConditionFalse, chantico.ReasonApplyFailed)
}

// TODO: CHECK
func TestReconcileValidation_UnsupportedType(t *testing.T) {
	measurementDevice := &chantico.MeasurementDevice{
		ObjectMeta: metav1.ObjectMeta{Name: testDeviceA, Namespace: testNamespace},
	}
	_, reconciler := setupPhysicalMeasurementTest(t, measurementDevice)

	for name, measurementType := range map[string]chantico.PhysicalMeasurementType{
		"unset":   "",
		"unknown": "modbus",
	} {
		t.Run(name, func(t *testing.T) {
			physicalMeasurement := &chantico.PhysicalMeasurement{
				ObjectMeta: metav1.ObjectMeta{Name: testPM, Namespace: testNamespace},
				Spec: chantico.PhysicalMeasurementSpec{
					MeasurementDevice: testDeviceA,
					Ip:                testIpA,
					Type:              measurementType,
				},
			}

			res := reconciler.reconcileValidation(t.Context(), physicalMeasurement)
			if res.Action != steps.ActionError {
				t.Fatalf("expected Error, got %v", res.Action)
			}
			assertPhysicalMeasurementCondition(t, physicalMeasurement, chantico.ConditionValidated, metav1.ConditionFalse, chantico.ReasonInvalidSpec)
		})
	}
}

func TestReconcileDeletion_RemovesScrapeConfig(t *testing.T) {
	client, reconciler := setupPhysicalMeasurementTest(t)

	now := metav1.Now()
	physicalMeasurement := &chantico.PhysicalMeasurement{
		ObjectMeta: metav1.ObjectMeta{
			Name:              testPM,
			UID:               types.UID("18ac6360-39e7-4ee3-a9b8-58992958e29a"),
			DeletionTimestamp: &now,
			Finalizers:        []string{chantico.PhysicalMeasurementFinalizer},
		},
		Spec: chantico.PhysicalMeasurementSpec{
			MeasurementDevice: testDeviceA,
			Ip:                testIpA,
			Type:              chantico.PhysicalMeasurementTypeSNMP,
		},
	}

	if res := reconciler.reconcileTargets(t.Context(), physicalMeasurement); res.Action == steps.ActionError {
		t.Fatalf("reconcileTargets errored: %v", res.Err)
	}
	assertScrapeConfigExistence(t, client, physicalMeasurement, true)

	res := reconciler.reconcileDeletion(t.Context(), physicalMeasurement)
	if res.Action == steps.ActionError {
		t.Fatalf("reconcileDeletion errored: %v", res.Err)
	}
	if res.Action != steps.ActionStop {
		t.Fatalf("expected Stop, got %v", res.Action)
	}
	assertScrapeConfigExistence(t, client, physicalMeasurement, false)
	if controllerutil.ContainsFinalizer(physicalMeasurement, chantico.PhysicalMeasurementFinalizer) {
		t.Fatalf("expected finalizer %q to be removed", chantico.PhysicalMeasurementFinalizer)
	}
}

func TestReconcilePhysicalMeasurementDeletion_NonExistentScrapeConfig(t *testing.T) {
	_, reconciler := setupPhysicalMeasurementTest(t)

	now := metav1.Now()
	physicalMeasurement := &chantico.PhysicalMeasurement{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "nonexistent",
			DeletionTimestamp: &now,
			Finalizers:        []string{chantico.PhysicalMeasurementFinalizer},
		},
		Spec: chantico.PhysicalMeasurementSpec{Type: chantico.PhysicalMeasurementTypeSNMP},
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
	client, reconciler := setupPhysicalMeasurementTest(t)

	physicalMeasurement := &chantico.PhysicalMeasurement{
		ObjectMeta: metav1.ObjectMeta{
			Name:       testPM,
			Finalizers: []string{chantico.PhysicalMeasurementFinalizer},
		},
		Spec: chantico.PhysicalMeasurementSpec{
			MeasurementDevice: testDeviceA,
			Ip:                testIpA,
			Type:              chantico.PhysicalMeasurementTypeSNMP,
		},
	}
	if res := reconciler.reconcileTargets(t.Context(), physicalMeasurement); res.Action == steps.ActionError {
		t.Fatalf("reconcileTargets errored: %v", res.Err)
	}

	res := reconciler.reconcileDeletion(t.Context(), physicalMeasurement)
	if res.Action != steps.ActionContinue {
		t.Fatalf("expected Continue, got %v", res.Action)
	}
	assertScrapeConfigExistence(t, client, physicalMeasurement, true)
}

func TestEnsurePhysicalMeasurementFinalizerIsSet(t *testing.T) {
	_, reconciler := setupPhysicalMeasurementTest(t)

	physicalMeasurement := &chantico.PhysicalMeasurement{
		ObjectMeta: metav1.ObjectMeta{Name: testPM},
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
		ObjectMeta: metav1.ObjectMeta{Name: testPM},
	}

	res := reconciler.reconcileReady(t.Context(), physicalMeasurement)
	if res.Action != steps.ActionContinue {
		t.Fatalf("expected Continue, got %v", res.Action)
	}
	assertPhysicalMeasurementCondition(t, physicalMeasurement, chantico.ConditionReady, metav1.ConditionTrue, chantico.ReasonReconciled)
}

func TestReconcilePhysicalMeasurementValidation_ExistingDevice(t *testing.T) {
	measurementDevice := &chantico.MeasurementDevice{
		ObjectMeta: metav1.ObjectMeta{Name: testDeviceA, Namespace: testNamespace},
	}
	_, reconciler := setupPhysicalMeasurementTest(t, measurementDevice)

	physicalMeasurement := &chantico.PhysicalMeasurement{
		ObjectMeta: metav1.ObjectMeta{Name: testPM, Namespace: testNamespace},
		Spec: chantico.PhysicalMeasurementSpec{
			MeasurementDevice: testDeviceA,
			Ip:                testIpA,
			Type:              chantico.PhysicalMeasurementTypeSNMP,
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
		ObjectMeta: metav1.ObjectMeta{Name: testPM, Namespace: testNamespace},
		Spec: chantico.PhysicalMeasurementSpec{
			MeasurementDevice: "does-not-exist",
			Ip:                testIpA,
			Type:              chantico.PhysicalMeasurementTypeSNMP,
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
		ObjectMeta: metav1.ObjectMeta{Name: testDeviceA, Namespace: "other"},
	}
	_, reconciler := setupPhysicalMeasurementTest(t, measurementDevice)

	physicalMeasurement := &chantico.PhysicalMeasurement{
		ObjectMeta: metav1.ObjectMeta{Name: testPM, Namespace: testNamespace},
		Spec: chantico.PhysicalMeasurementSpec{
			MeasurementDevice: testDeviceA,
			Ip:                testIpA,
			Type:              chantico.PhysicalMeasurementTypeSNMP,
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

func assertScrapeConfigExistence(t *testing.T, client client.WithWatch, resource *chantico.PhysicalMeasurement, exists bool) {
	t.Helper()

	_, err := readScrapeConfigResource(t, client, resource)
	if exists && err != nil {
		t.Fatalf("expected ScrapeConfig to exist for resource %s: %v", resource.Name, err)
	}
	if !exists && err == nil {
		t.Fatalf("expected ScrapeConfig to not exist for resource %s, but it does", resource.Name)
	}
}

func fetchScrapeConfigResource(t *testing.T, r *chantico.PhysicalMeasurement, client client.WithWatch) (*monitoringv1alpha1.ScrapeConfig, error) {
	config := &monitoringv1alpha1.ScrapeConfig{}
	key := types.NamespacedName{Namespace: r.Namespace, Name: r.Name}
	if err := client.Get(t.Context(), key, config); err != nil {
		return nil, fmt.Errorf("fetching ScrapeConfig: %w", err)
	}
	return config, nil
}

func readScrapeConfigResource(t *testing.T, client client.WithWatch, resource *chantico.PhysicalMeasurement) (*monitoringv1alpha1.ScrapeConfig, error) {
	t.Helper()

	configResource, err := fetchScrapeConfigResource(t, resource, client)
	if err != nil {
		return nil, fmt.Errorf("fetch scrape config resource: %w", err)
	}

	return configResource, nil
}

func writeScrapeConfigResource(t *testing.T, client client.WithWatch, resource *chantico.PhysicalMeasurement, config *monitoringv1alpha1.ScrapeConfig) {
	t.Helper()

	if err := client.Create(t.Context(), config); err != nil {
		t.Fatalf("creating ScrapeConfig: %v", err)
	}
}
