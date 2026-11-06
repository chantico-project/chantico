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
	"fmt"
	"reflect"
	"testing"

	chantico "chantico/api/v1alpha1"
	dcr "chantico/internal/datacenterresource"
	"chantico/internal/k8s"
	"chantico/internal/steps"
	"chantico/internal/testutil"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func setupDataCenterResourceRuleTest(t *testing.T) (client.WithWatch, *DataCenterResourceReconciler) {
	t.Helper()

	kubeClient := testutil.NewFakeClientBuilder(t, chantico.AddToScheme, monitoringv1.AddToScheme).
		WithStatusSubresource(&chantico.DataCenterResource{}).
		Build()

	ruleConfigurer := dcr.PrometheusRecordingRuleConfigurer{
		Client: kubeClient,
		Scheme: kubeClient.Scheme(),
	}

	return kubeClient, &DataCenterResourceReconciler{
		Client:         kubeClient,
		RuleConfigurer: &ruleConfigurer,
	}
}

func fetchRuleResource(t *testing.T, r *chantico.DataCenterResource, client client.WithWatch) (*monitoringv1.PrometheusRule, error) {
	rule := &monitoringv1.PrometheusRule{}
	key := types.NamespacedName{Namespace: r.Namespace, Name: r.Name}
	if err := client.Get(t.Context(), key, rule); err != nil {
		return nil, fmt.Errorf("fetching PrometheusRule: %w", err)
	}
	return rule, nil
}

func TestNormalizeDataCenterResource(t *testing.T) {
	explicit := &k8s.TemplateFrom{
		SecretConfigMapSelector: k8s.SecretConfigMapSelector{Value: "existing_expression"},
	}
	tests := []struct {
		name string
		spec chantico.DataCenterResourceSpec
	}{
		{name: "energy metric", spec: chantico.DataCenterResourceSpec{EnergyMetric: "pdu_power_watts"}},
		{name: "coefficients", spec: chantico.DataCenterResourceSpec{Parents: []chantico.ParentRef{
			{Name: "pdu1", Coefficient: "0.5"},
			{Name: "pdu2", Coefficient: "0"},
			{Name: "pdu3", CoefficientFrom: explicit},
		}}},
		{name: "explicit template", spec: chantico.DataCenterResourceSpec{EnergyMetricFrom: explicit}},
		{name: "empty", spec: chantico.DataCenterResourceSpec{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resource := &chantico.DataCenterResource{Spec: test.spec}
			normalizeDataCenterResource(resource)
			if test.spec.EnergyMetric != "" {
				if resource.Spec.EnergyMetricFrom == nil || resource.Spec.EnergyMetricFrom.Value != test.spec.EnergyMetric {
					t.Fatalf("energy metric was not expanded: %#v", resource.Spec.EnergyMetricFrom)
				}
			} else if !reflect.DeepEqual(resource.Spec.EnergyMetricFrom, test.spec.EnergyMetricFrom) {
				t.Fatal("explicit energy metric template changed")
			}
			for _, parent := range resource.Spec.Parents {
				if parent.Coefficient != "" {
					if parent.CoefficientFrom == nil || parent.CoefficientFrom.Value != parent.Coefficient {
						t.Fatalf("coefficient for %s was not expanded: %#v", parent.Name, parent.CoefficientFrom)
					}
				} else if !reflect.DeepEqual(parent.CoefficientFrom, explicit) {
					t.Fatal("explicit coefficient template changed")
				}
			}
			first := resource.DeepCopy()
			normalizeDataCenterResource(resource)
			if !reflect.DeepEqual(resource, first) {
				t.Fatal("normalization is not idempotent")
			}
		})
	}
}

func TestReconcileWriteRule_NonRootNode(t *testing.T) {
	client, reconciler := setupDataCenterResourceRuleTest(t)

	bm := &chantico.DataCenterResource{
		ObjectMeta: metav1.ObjectMeta{Name: "bm1"},
		Spec: chantico.DataCenterResourceSpec{
			Type: dcr.DataCenterResourceTypeBaremetal,
			Parents: []chantico.ParentRef{
				{Name: "pdu1", CoefficientFrom: &k8s.TemplateFrom{
					SecretConfigMapSelector: k8s.SecretConfigMapSelector{
						Value: "0.6",
					},
				}},
			},
		},
	}

	res := reconciler.reconcileWriteRule(t.Context(), bm)
	if res.Action == steps.ActionError {
		t.Fatalf("reconcileWriteRule errored: %v", res.Err)
	}
	if res.Action != steps.ActionContinue {
		t.Fatalf("expected Continue, got %v", res.Action)
	}

	prometheusRule, err := fetchRuleResource(t, bm, client)
	if err != nil {
		t.Fatalf("fetching PrometheusRule: %v", err)
	}
	groups := prometheusRule.Spec.Groups

	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if groups[0].Name != "chantico_"+bm.Name {
		t.Errorf("expected group name %q, got %q", "chantico_"+bm.Name, groups[0].Name)
	}
	if len(groups[0].Rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(groups[0].Rules))
	}
	assertDataCenterResourceCondition(t, bm, chantico.ConditionApplied, metav1.ConditionTrue, chantico.ReasonReconciled)
}

func TestReconcileWriteRule_RootNodeWithEnergyMetric(t *testing.T) {
	client, reconciler := setupDataCenterResourceRuleTest(t)

	pdu := &chantico.DataCenterResource{
		ObjectMeta: metav1.ObjectMeta{Name: "pdu1"},
		Spec: chantico.DataCenterResourceSpec{
			Type: dcr.DataCenterResourceTypePDU,
			EnergyMetricFrom: &k8s.TemplateFrom{
				SecretConfigMapSelector: k8s.SecretConfigMapSelector{
					Value: "snmp_pdu1_power_watts",
				},
			},
		},
	}

	res := reconciler.reconcileWriteRule(t.Context(), pdu)
	if res.Action == steps.ActionError {
		t.Fatalf("reconcileWriteRule errored: %v", res.Err)
	}

	_, err := fetchRuleResource(t, pdu, client)
	if err != nil {
		t.Fatalf("expected existing PrometheusRule but got error: %v", err)
	}
	assertDataCenterResourceCondition(t, pdu, chantico.ConditionApplied, metav1.ConditionTrue, chantico.ReasonReconciled)
}

func TestReconcileWriteRule_OverwritesExisting(t *testing.T) {
	client, reconciler := setupDataCenterResourceRuleTest(t)

	bm := &chantico.DataCenterResource{
		ObjectMeta: metav1.ObjectMeta{Name: "bm1"},
		Spec: chantico.DataCenterResourceSpec{
			Type: dcr.DataCenterResourceTypeBaremetal,
			Parents: []chantico.ParentRef{
				{Name: "pdu1", CoefficientFrom: &k8s.TemplateFrom{
					SecretConfigMapSelector: k8s.SecretConfigMapSelector{
						Value: "0.5",
					},
				}},
			},
		},
	}

	if res := reconciler.reconcileWriteRule(t.Context(), bm); res.Action == steps.ActionError {
		t.Fatalf("first reconcileWriteRule errored: %v", res.Err)
	}
	bm.Spec.Parents[0].Name = "pdu2"
	if res := reconciler.reconcileWriteRule(t.Context(), bm); res.Action == steps.ActionError {
		t.Fatalf("second reconcileWriteRule errored: %v", res.Err)
	}

	data, err := fetchRuleResource(t, bm, client)
	if err != nil {
		t.Fatalf("expected existing PrometheusRule but got error: %v", err)
	}
	assertDataCenterResourceCondition(t, bm, chantico.ConditionApplied, metav1.ConditionTrue, chantico.ReasonReconciled)

	if len(data.Spec.Groups) != 1 {
		t.Fatalf("expected 1 group in PrometheusRule, got %d", len(data.Spec.Groups))
	}

	found := false
	for _, rule := range data.Spec.Groups[0].Rules {
		if rule.Record == "chantico_power_coefficient" && rule.Labels["parent"] == "pdu2" {
			found = true
		}
		if rule.Labels["parent"] == "pdu1" {
			t.Errorf("expected stale parent 'pdu1' to be replaced, got rule %+v", rule)
		}
	}
	if !found {
		t.Errorf("expected coefficient rule with parent label 'pdu2', got %+v", data.Spec.Groups[0].Rules)
	}
}

func TestReconcileDeletion_RemovesRuleFile(t *testing.T) {
	client, reconciler := setupDataCenterResourceRuleTest(t)

	promRule := &monitoringv1.PrometheusRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bm1",
			Namespace: "default",
		},
		Spec: monitoringv1.PrometheusRuleSpec{
			Groups: []monitoringv1.RuleGroup{
				{
					Name:  "bm1",
					Rules: []monitoringv1.Rule{},
				},
			},
		},
	}
	writePrometheusRuleTestResource(t, client, promRule)

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
	if bm.DeletionTimestamp == nil {
		t.Fatalf("expected DeletionTimestamp to be set")
	}

	assertPrometheusRuleExistence(t, client, bm, false)
	if controllerutil.ContainsFinalizer(bm, chantico.DataCenterResourceGraphFinalizer) {
		t.Fatalf("expected finalizer %q to be removed", chantico.DataCenterResourceGraphFinalizer)
	}
}

func TestReconcileDeletion_NonExistentRuleFile(t *testing.T) {
	_, reconciler := setupDataCenterResourceRuleTest(t)
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
}

func TestReconcileWriteRule_FullThreeLayerHierarchy(t *testing.T) {
	client, reconciler := setupDataCenterResourceRuleTest(t)

	pdu1 := &chantico.DataCenterResource{
		ObjectMeta: metav1.ObjectMeta{Name: "pdu1"},
		Spec: chantico.DataCenterResourceSpec{
			Type: dcr.DataCenterResourceTypePDU,
			EnergyMetricFrom: &k8s.TemplateFrom{
				SecretConfigMapSelector: k8s.SecretConfigMapSelector{
					Value: "snmp_pdu1_power_watts",
				},
			},
		},
	}
	bm1 := &chantico.DataCenterResource{
		ObjectMeta: metav1.ObjectMeta{Name: "bm1"},
		Spec: chantico.DataCenterResourceSpec{
			Type: dcr.DataCenterResourceTypeBaremetal,
			Parents: []chantico.ParentRef{
				{Name: "pdu1", CoefficientFrom: &k8s.TemplateFrom{
					SecretConfigMapSelector: k8s.SecretConfigMapSelector{
						Value: "1",
					},
				}},
			},
		},
	}
	vm1 := &chantico.DataCenterResource{
		ObjectMeta: metav1.ObjectMeta{Name: "vm1"},
		Spec: chantico.DataCenterResourceSpec{
			Type: dcr.DataCenterResourceTypeVM,
			Parents: []chantico.ParentRef{
				{Name: "bm1", CoefficientFrom: &k8s.TemplateFrom{
					SecretConfigMapSelector: k8s.SecretConfigMapSelector{
						Value: "0.4",
					},
				}},
			},
		},
	}
	vm2 := &chantico.DataCenterResource{
		ObjectMeta: metav1.ObjectMeta{Name: "vm2"},
		Spec: chantico.DataCenterResourceSpec{
			Type: dcr.DataCenterResourceTypeVM,
			Parents: []chantico.ParentRef{
				{Name: "bm1", CoefficientFrom: &k8s.TemplateFrom{
					SecretConfigMapSelector: k8s.SecretConfigMapSelector{
						Value: "0.6",
					},
				}},
			},
		},
	}

	for _, resource := range []*chantico.DataCenterResource{pdu1, bm1, vm1, vm2} {
		if res := reconciler.reconcileWriteRule(t.Context(), resource); res.Action == steps.ActionError {
			t.Fatalf("reconcileWriteRule(%s) errored: %v", resource.Name, res.Err)
		}
	}

	assertPrometheusRuleExistence(t, client, pdu1, true)
	assertPrometheusRuleExistence(t, client, bm1, true)
	assertPrometheusRuleExistence(t, client, vm1, true)
	assertPrometheusRuleExistence(t, client, vm2, true)

	pdu1Rule, err := readRuleResource(t, client, pdu1)
	if err != nil {
		t.Fatalf("PDU1: %v", err)
	}
	if len(pdu1Rule.Spec.Groups[0].Rules) != 1 {
		t.Errorf("PDU1: expected 1 alias rule, got %d", len(pdu1Rule.Spec.Groups[0].Rules))
	}

	bm1Rule, err := readRuleResource(t, client, bm1)
	if err != nil {
		t.Fatalf("BM1: %v", err)
	}
	if len(bm1Rule.Spec.Groups[0].Rules) != 2 {
		t.Errorf("BM1: expected 2 rules (1 coefficient + 1 energy), got %d", len(bm1Rule.Spec.Groups[0].Rules))
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

func readRuleResource(t *testing.T, client client.WithWatch, resource *chantico.DataCenterResource) (*monitoringv1.PrometheusRule, error) {
	t.Helper()

	ruleResource, err := fetchRuleResource(t, resource, client)
	if err != nil {
		return nil, fmt.Errorf("fetch rule resource: %w", err)
	}

	return ruleResource, nil
}

func assertPrometheusRuleExistence(t *testing.T, client client.WithWatch, resource *chantico.DataCenterResource, exists bool) {
	t.Helper()

	_, err := readRuleResource(t, client, resource)
	if exists && err != nil {
		t.Fatalf("expected Prometheus rule to exist for resource %s: %v", resource.Name, err)
	}
	if !exists && err == nil {
		t.Fatalf("expected Prometheus rule to not exist for resource %s, but it does", resource.Name)
	}
}

func writePrometheusRuleTestResource(t *testing.T, client client.WithWatch, rule *monitoringv1.PrometheusRule) {
	t.Helper()

	if err := client.Create(t.Context(), rule); err != nil {
		t.Fatalf("creating PrometheusRule: %v", err)
	}
}
