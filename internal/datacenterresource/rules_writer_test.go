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

package datacenterresource

import (
	"testing"

	chantico "chantico/api/v1alpha1"
	"chantico/internal/k8s"
	"chantico/internal/testutil"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func setupRecordingRulesTest(t *testing.T) (*KubePrometheusRecordingRules, *chantico.DataCenterResource) {
	t.Helper()

	kubeClient := testutil.NewFakeClientBuilder(t, monitoringv1.AddToScheme).Build()
	rules := &KubePrometheusRecordingRules{Client: kubeClient}
	resource := &chantico.DataCenterResource{
		ObjectMeta: metav1.ObjectMeta{Name: "pdu", Namespace: "chantico"},
		Spec: chantico.DataCenterResourceSpec{
			Type: DataCenterResourceTypePDU,
			EnergyMetricFrom: &k8s.TemplateFrom{
				SecretConfigMapSelector: k8s.SecretConfigMapSelector{Value: "raw_power_watts"},
			},
		},
	}
	return rules, resource
}

func reconcileRecordingRulesForTest(t *testing.T, rules *KubePrometheusRecordingRules, resource *chantico.DataCenterResource) *monitoringv1.PrometheusRule {
	t.Helper()

	if err := rules.Reconcile(t.Context(), resource); err != nil {
		t.Fatal(err)
	}

	observed := &monitoringv1.PrometheusRule{}
	key := client.ObjectKey{Namespace: resource.Namespace, Name: resource.Name}
	if err := rules.Client.Get(t.Context(), key, observed); err != nil {
		t.Fatal(err)
	}
	return observed
}

func TestKubePrometheusRecordingRulesReconcile_CreatesAndUpdatesRule(t *testing.T) {
	rules, resource := setupRecordingRulesTest(t)
	observed := reconcileRecordingRulesForTest(t, rules, resource)

	if observed.Labels["app.kubernetes.io/managed-by"] != "chantico" {
		t.Fatalf("unexpected managed-by label: %v", observed.Labels)
	}
	if len(observed.Spec.Groups) != 1 || observed.Spec.Groups[0].Name != "chantico_pdu" {
		t.Fatalf("unexpected rule groups: %#v", observed.Spec.Groups)
	}
	if len(observed.Spec.Groups[0].Rules) != 1 {
		t.Fatalf("expected one recording rule, got %d", len(observed.Spec.Groups[0].Rules))
	}
	rule := observed.Spec.Groups[0].Rules[0]
	if rule.Record != EnergyMetricName || rule.Expr.String() != "raw_power_watts" {
		t.Fatalf("unexpected recording rule: %#v", rule)
	}

	resource.Spec.EnergyMetricFrom.Value = "updated_power_watts"
	updated := reconcileRecordingRulesForTest(t, rules, resource)
	if got := updated.Spec.Groups[0].Rules[0].Expr.String(); got != "updated_power_watts" {
		t.Fatalf("updated expression = %q, want %q", got, "updated_power_watts")
	}
}

func TestKubePrometheusRecordingRulesReconcile_SkipsUnchangedRule(t *testing.T) {
	rules, resource := setupRecordingRulesTest(t)
	observed := reconcileRecordingRulesForTest(t, rules, resource)
	unchanged := reconcileRecordingRulesForTest(t, rules, resource)

	if unchanged.ResourceVersion != observed.ResourceVersion {
		t.Fatal("unchanged reconciliation updated the PrometheusRule")
	}
}

func TestKubePrometheusRecordingRulesDelete_MissingRule(t *testing.T) {
	rules, resource := setupRecordingRulesTest(t)
	if err := rules.Delete(t.Context(), resource); err != nil {
		t.Fatalf("deleting an absent PrometheusRule failed: %v", err)
	}
}
