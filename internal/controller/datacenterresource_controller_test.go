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
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	chantico "chantico/api/v1alpha1"
	config "chantico/internal/configuration"
	dcr "chantico/internal/datacenterresource"
	"chantico/internal/steps"

	"go.yaml.in/yaml/v2"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func setupDataCenterResourceRuleTest(t *testing.T) (string, *DataCenterResourceReconciler, *atomic.Int32) {
	t.Helper()

	tmpDir := t.TempDir()
	reloads := &atomic.Int32{}
	server := newReloadServer(t, reloads)
	t.Cleanup(server.Close)

	host, port := serverHostPort(t, server)
	t.Setenv(config.ChanticoVolumeLocationEnv, tmpDir)
	t.Setenv(config.ChanticoVolumeClaimEnv, "chantico-snmp-prometheus-volume-claim")
	t.Setenv(config.ChanticoPrometheusServiceHostEnv, host)
	t.Setenv(config.ChanticoPrometheusServicePortEnv, port)

	var errs []error
	config.ValidatedEnv, errs = config.ValidateEnv()
	if len(errs) > 0 {
		t.Fatalf("validating test environment: %v", errs)
	}

	rulesDir := filepath.Join(tmpDir, prometheusRulesDir)
	if err := os.MkdirAll(rulesDir, 0755); err != nil {
		t.Fatalf("create rules directory %s: %v", rulesDir, err)
	}

	return tmpDir, &DataCenterResourceReconciler{}, reloads
}

func newReloadServer(t *testing.T, reloads *atomic.Int32) *httptest.Server {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for mock Prometheus: %v", err)
	}

	server := &httptest.Server{
		Listener: listener,
		Config: &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			if r.URL.Path != "/-/reload" {
				http.NotFound(w, r)
				return
			}
			reloads.Add(1)
			w.WriteHeader(http.StatusOK)
		})},
	}
	server.Start()

	return server
}

func serverHostPort(t *testing.T, server *httptest.Server) (string, string) {
	t.Helper()

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server URL %q: %v", server.URL, err)
	}
	host, port, err := net.SplitHostPort(serverURL.Host)
	if err != nil {
		t.Fatalf("split server host %q: %v", serverURL.Host, err)
	}
	return host, port
}

func TestReconcileWriteRuleFile_NonRootNode(t *testing.T) {
	tmpDir, reconciler, reloads := setupDataCenterResourceRuleTest(t)

	bm := &chantico.DataCenterResource{
		ObjectMeta: metav1.ObjectMeta{Name: "bm1"},
		Spec: chantico.DataCenterResourceSpec{
			Type: dcr.DataCenterResourceTypeBaremetal,
			Parents: []chantico.ParentRef{
				{Name: "pdu1", Coefficient: "0.6"},
			},
		},
	}

	res := reconciler.reconcileWriteRuleFile(t.Context(), bm)
	if res.Action == steps.ActionError {
		t.Fatalf("reconcileWriteRuleFile errored: %v", res.Err)
	}
	if res.Action != steps.ActionContinue {
		t.Fatalf("expected Continue, got %v", res.Action)
	}

	rulePath := filepath.Join(tmpDir, prometheusRulesDir, "bm1.yml")
	data, err := os.ReadFile(rulePath)
	if err != nil {
		t.Fatalf("expected rule file to exist at %s: %v", rulePath, err)
	}

	var ruleFile dcr.RuleFile
	if err := yaml.Unmarshal(data, &ruleFile); err != nil {
		t.Fatalf("parse rule file: %v", err)
	}

	if len(ruleFile.Groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(ruleFile.Groups))
	}
	if ruleFile.Groups[0].Name != "chantico_bm1" {
		t.Errorf("expected group name %q, got %q", "chantico_bm1", ruleFile.Groups[0].Name)
	}
	if len(ruleFile.Groups[0].Rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(ruleFile.Groups[0].Rules))
	}
	assertDataCenterResourceCondition(t, bm, chantico.ConditionApplied, metav1.ConditionTrue, chantico.ReasonReconciled)
	assertReloads(t, reloads, 1)
}

func TestReconcileWriteRuleFile_RootNodeWithEnergyMetric(t *testing.T) {
	tmpDir, reconciler, reloads := setupDataCenterResourceRuleTest(t)

	pdu := &chantico.DataCenterResource{
		ObjectMeta: metav1.ObjectMeta{Name: "pdu1"},
		Spec: chantico.DataCenterResourceSpec{
			Type:         dcr.DataCenterResourceTypePDU,
			EnergyMetric: "snmp_pdu1_power_watts",
		},
	}

	res := reconciler.reconcileWriteRuleFile(t.Context(), pdu)
	if res.Action == steps.ActionError {
		t.Fatalf("reconcileWriteRuleFile errored: %v", res.Err)
	}

	rulePath := filepath.Join(tmpDir, prometheusRulesDir, "pdu1.yml")
	if _, err := os.Stat(rulePath); err != nil {
		t.Fatalf("expected rule file to exist at %s: %v", rulePath, err)
	}
	assertDataCenterResourceCondition(t, pdu, chantico.ConditionApplied, metav1.ConditionTrue, chantico.ReasonReconciled)
	assertReloads(t, reloads, 1)
}

func TestReconcileWriteRuleFile_OverwritesExisting(t *testing.T) {
	tmpDir, reconciler, reloads := setupDataCenterResourceRuleTest(t)

	bm := &chantico.DataCenterResource{
		ObjectMeta: metav1.ObjectMeta{Name: "bm1"},
		Spec: chantico.DataCenterResourceSpec{
			Type: dcr.DataCenterResourceTypeBaremetal,
			Parents: []chantico.ParentRef{
				{Name: "pdu1", Coefficient: "0.5"},
			},
		},
	}

	if res := reconciler.reconcileWriteRuleFile(t.Context(), bm); res.Action == steps.ActionError {
		t.Fatalf("first reconcileWriteRuleFile errored: %v", res.Err)
	}
	bm.Spec.Parents[0].Coefficient = "0.8"
	if res := reconciler.reconcileWriteRuleFile(t.Context(), bm); res.Action == steps.ActionError {
		t.Fatalf("second reconcileWriteRuleFile errored: %v", res.Err)
	}

	rulePath := filepath.Join(tmpDir, prometheusRulesDir, "bm1.yml")
	data, err := os.ReadFile(rulePath)
	if err != nil {
		t.Fatalf("expected rule file to exist: %v", err)
	}
	if !strings.Contains(string(data), "0.8") {
		t.Errorf("expected updated coefficient 0.8 in rule file, got:\n%s", string(data))
	}
	assertReloads(t, reloads, 2)
}

func TestReconcileDeletion_RemovesRuleFile(t *testing.T) {
	tmpDir, reconciler, reloads := setupDataCenterResourceRuleTest(t)
	rulePath := filepath.Join(tmpDir, prometheusRulesDir, "bm1.yml")
	writeDCRTestFile(t, rulePath, []byte("groups: []\n"))

	now := metav1.Now()
	bm := &chantico.DataCenterResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "bm1",
			DeletionTimestamp: &now,
			Finalizers:        []string{chantico.DataCenterResourceGraphFinalizer},
		},
	}

	res := reconciler.reconcileDeletion(t.Context(), bm)
	if res.Action == steps.ActionError {
		t.Fatalf("reconcileDeletion errored: %v", res.Err)
	}
	if res.Action != steps.ActionStop {
		t.Fatalf("expected Stop, got %v", res.Action)
	}
	if _, err := os.Stat(rulePath); !os.IsNotExist(err) {
		t.Fatalf("expected rule file to be deleted, stat err = %v", err)
	}
	if controllerutil.ContainsFinalizer(bm, chantico.DataCenterResourceGraphFinalizer) {
		t.Fatalf("expected finalizer %q to be removed", chantico.DataCenterResourceGraphFinalizer)
	}
	assertReloads(t, reloads, 1)
}

func TestReconcileDeletion_NonExistentRuleFile(t *testing.T) {
	_, reconciler, reloads := setupDataCenterResourceRuleTest(t)
	now := metav1.Now()
	bm := &chantico.DataCenterResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "nonexistent",
			DeletionTimestamp: &now,
			Finalizers:        []string{chantico.DataCenterResourceGraphFinalizer},
		},
	}

	res := reconciler.reconcileDeletion(t.Context(), bm)
	if res.Action == steps.ActionError {
		t.Fatalf("reconcileDeletion errored: %v", res.Err)
	}
	if res.Action != steps.ActionStop {
		t.Fatalf("expected Stop, got %v", res.Action)
	}
	assertReloads(t, reloads, 1)
}

func TestReconcileWriteRuleFile_FullThreeLayerHierarchy(t *testing.T) {
	tmpDir, reconciler, _ := setupDataCenterResourceRuleTest(t)

	pdu1 := &chantico.DataCenterResource{
		ObjectMeta: metav1.ObjectMeta{Name: "pdu1"},
		Spec: chantico.DataCenterResourceSpec{
			Type:         dcr.DataCenterResourceTypePDU,
			EnergyMetric: "snmp_pdu1_power_watts",
		},
	}
	bm1 := &chantico.DataCenterResource{
		ObjectMeta: metav1.ObjectMeta{Name: "bm1"},
		Spec: chantico.DataCenterResourceSpec{
			Type: dcr.DataCenterResourceTypeBaremetal,
			Parents: []chantico.ParentRef{
				{Name: "pdu1", Coefficient: "1"},
			},
		},
	}
	vm1 := &chantico.DataCenterResource{
		ObjectMeta: metav1.ObjectMeta{Name: "vm1"},
		Spec: chantico.DataCenterResourceSpec{
			Type: dcr.DataCenterResourceTypeVM,
			Parents: []chantico.ParentRef{
				{Name: "bm1", Coefficient: "0.4"},
			},
		},
	}
	vm2 := &chantico.DataCenterResource{
		ObjectMeta: metav1.ObjectMeta{Name: "vm2"},
		Spec: chantico.DataCenterResourceSpec{
			Type: dcr.DataCenterResourceTypeVM,
			Parents: []chantico.ParentRef{
				{Name: "bm1", Coefficient: "0.6"},
			},
		},
	}

	for _, resource := range []*chantico.DataCenterResource{pdu1, bm1, vm1, vm2} {
		if res := reconciler.reconcileWriteRuleFile(t.Context(), resource); res.Action == steps.ActionError {
			t.Fatalf("reconcileWriteRuleFile(%s) errored: %v", resource.Name, res.Err)
		}
	}

	rulesDir := filepath.Join(tmpDir, prometheusRulesDir)
	assertDCRRuleFileExists(t, rulesDir, "pdu1.yml")
	assertDCRRuleFileExists(t, rulesDir, "bm1.yml")
	assertDCRRuleFileExists(t, rulesDir, "vm1.yml")
	assertDCRRuleFileExists(t, rulesDir, "vm2.yml")

	pdu1RuleFile := readDCRRuleFile(t, filepath.Join(rulesDir, "pdu1.yml"))
	if len(pdu1RuleFile.Groups[0].Rules) != 1 {
		t.Errorf("PDU1: expected 1 alias rule, got %d", len(pdu1RuleFile.Groups[0].Rules))
	}

	bm1RuleFile := readDCRRuleFile(t, filepath.Join(rulesDir, "bm1.yml"))
	if len(bm1RuleFile.Groups[0].Rules) != 2 {
		t.Errorf("BM1: expected 2 rules (1 coefficient + 1 energy), got %d", len(bm1RuleFile.Groups[0].Rules))
	}
}

func assertDataCenterResourceCondition(t *testing.T, resource *chantico.DataCenterResource, conditionType chantico.ConditionType, status metav1.ConditionStatus, reason chantico.ConditionReason) {
	t.Helper()

	condition := meta.FindStatusCondition(resource.Status.Conditions, string(conditionType))
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

func assertReloads(t *testing.T, reloads *atomic.Int32, want int32) {
	t.Helper()

	if got := reloads.Load(); got != want {
		t.Fatalf("expected %d Prometheus reloads, got %d", want, got)
	}
}

func assertDCRRuleFileExists(t *testing.T, rulesDir, filename string) {
	t.Helper()

	path := filepath.Join(rulesDir, filename)
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected rule file %s to exist: %v", filename, err)
	}
}

func readDCRRuleFile(t *testing.T, path string) dcr.RuleFile {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read rule file %s: %v", path, err)
	}
	var ruleFile dcr.RuleFile
	if err := yaml.Unmarshal(data, &ruleFile); err != nil {
		t.Fatalf("parse rule file %s: %v", path, err)
	}
	return ruleFile
}

func writeDCRTestFile(t *testing.T, path string, data []byte) {
	t.Helper()

	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
