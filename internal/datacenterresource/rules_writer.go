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
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func BuildRuleFile(ctx context.Context, client client.Client, resource *chantico.DataCenterResource) (*RuleFile, error) {

	renderer := NewTemplateRenderer(client, resource.Namespace)
	resolvedExpressions, err := renderer.ResolvedExpressions(ctx, resource)
	if err != nil {
		return nil, err
	}

	return NewRuleBuilder(resource, resolvedExpressions).BuildRuleFile(), nil
}

type PrometheusRecordingRuleConfigurer struct {
	Client client.Client
	Scheme *runtime.Scheme
}

func buildRuleGroup(group *RuleGroup) monitoringv1.RuleGroup {
	promRules := []monitoringv1.Rule{}
	for _, rule := range group.Rules {
		promRules = append(promRules, monitoringv1.Rule{
			Expr:   intstr.FromString(rule.Expr),
			Labels: rule.Labels,
			Record: rule.Record,
		})
	}

	return monitoringv1.RuleGroup{
		Name:  group.Name,
		Rules: promRules,
	}
}

func (f *PrometheusRecordingRuleConfigurer) Reconcile(ctx context.Context, resource *chantico.DataCenterResource) error {
	ruleFile, err := BuildRuleFile(ctx, f.Client, resource)
	if err != nil {
		return err
	}
	if ruleFile == nil {
		return f.Delete(ctx, resource)
	}

	promGroups := []monitoringv1.RuleGroup{}
	for _, group := range ruleFile.Groups {
		promGroups = append(promGroups, buildRuleGroup(&group))
	}

	rule := &monitoringv1.PrometheusRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resource.Name,
			Namespace: resource.Namespace,
			Labels:    k8s.PrometheusResourceLabels(),
		},
		Spec: monitoringv1.PrometheusRuleSpec{
			Groups: promGroups,
		},
	}

	desired := rule.DeepCopy()

	if err := controllerutil.SetControllerReference(resource, rule, f.Scheme); err != nil {
		return fmt.Errorf("set controller reference: %w", err)
	}

	_, err = controllerutil.CreateOrUpdate(ctx, f.Client, rule, func() error {
		if rule.Labels == nil {
			rule.Labels = make(map[string]string)
		}
		for key, value := range desired.Labels {
			rule.Labels[key] = value
		}
		rule.Spec = desired.Spec
		return nil
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
