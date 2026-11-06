package datacenterresource

import (
	chantico "chantico/api/v1alpha1"
	"chantico/internal/k8s"
	"context"
	"fmt"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"k8s.io/apimachinery/pkg/runtime"
	intstr "k8s.io/apimachinery/pkg/util/intstr"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type PrometheusRecordingRuleConfigurer struct {
	Client client.Client
	Scheme *runtime.Scheme
}

func (f *PrometheusRecordingRuleConfigurer) Reconcile(ctx context.Context, resource *chantico.DataCenterResource) error {
	generatedRules, err := generateRules(ctx, f.Client, resource)
	if err != nil {
		return err
	}

	promRuleSpec := convertPrometheusRuleSpec(generatedRules)

	rule := &monitoringv1.PrometheusRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resource.Name,
			Namespace: resource.Namespace,
			Labels:    k8s.PrometheusResourceLabels(),
		},
		Spec: promRuleSpec,
	}

	err = k8s.ApplyWithOwner(ctx, f.Client, f.Scheme, resource, rule, func(existing, desired *monitoringv1.PrometheusRule) {
		existing.Spec = desired.Spec
	})
	if err != nil {
		return fmt.Errorf("reconcile prometheus rule: %w", err)
	}
	return nil
}

func (f *PrometheusRecordingRuleConfigurer) Delete(ctx context.Context, resource *chantico.DataCenterResource) error {

	scrapeConfig := &monitoringv1.PrometheusRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resource.Name,
			Namespace: resource.Namespace,
		},
	}
	if err := client.IgnoreNotFound(f.Client.Delete(ctx, scrapeConfig)); err != nil {
		return fmt.Errorf("delete scrape config: %w", err)
	}
	return nil
}

func generateRules(ctx context.Context, client client.Client, resource *chantico.DataCenterResource) (*RuleFile, error) {
	renderer := NewTemplateRenderer(client, resource.Namespace)
	resolvedExpressions, err := renderer.ResolvedExpressions(ctx, resource)
	if err != nil {
		return nil, err
	}

	return NewRuleBuilder(resource, resolvedExpressions).BuildRuleFile(), nil
}

func convertPrometheusRuleSpec(generatedRules *RuleFile) monitoringv1.PrometheusRuleSpec {
	promGroups := []monitoringv1.RuleGroup{}
	for _, group := range generatedRules.Groups {
		promRules := []monitoringv1.Rule{}
		for _, rule := range group.Rules {
			promRules = append(promRules, monitoringv1.Rule{
				Expr:   intstr.FromString(rule.Expr),
				Labels: rule.Labels,
				Record: rule.Record,
			})
		}
		promGroups = append(promGroups, monitoringv1.RuleGroup{
			Name:  group.Name,
			Rules: promRules,
		})
	}

	spec := monitoringv1.PrometheusRuleSpec{
		Groups: promGroups,
	}

	return spec
}
