package datacenterresource

import (
	chantico "chantico/api/v1alpha1"
	config "chantico/internal/configuration"
	"chantico/internal/k8s"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	intstr "k8s.io/apimachinery/pkg/util/intstr"

	"go.yaml.in/yaml/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type FileRecordingRules struct {
	Client client.Client
}

const prometheusRulesDir = "prometheus/rules"

func BuildRuleFile(f *FileRecordingRules, ctx context.Context, resource *chantico.DataCenterResource) (*RuleFile, error) {

	renderer := NewTemplateRenderer(f.Client, resource.Namespace)
	resolvedExpressions, err := renderer.ResolvedExpressions(ctx, resource)
	if err != nil {
		return nil, err
	}

	return NewRuleBuilder(resource, resolvedExpressions).BuildRuleFile(), nil
}

func (f *FileRecordingRules) Reconcile(ctx context.Context, resource *chantico.DataCenterResource) error {
	l := log.FromContext(ctx)

	ruleFile, err := BuildRuleFile(f, ctx, resource)
	if err != nil {
		return err
	}

	if ruleFile == nil {
		l.Info("No rule file found")
		if err := deleteRuleFile(resource); err != nil {
			return err
		}
		if err := f.reloadPrometheus(ctx); err != nil {
			return err
		}
		return nil
	}

	volumePath := config.ValidatedEnv.VolumeLocation
	rulesDir := filepath.Join(volumePath, prometheusRulesDir)
	if err := os.MkdirAll(rulesDir, 0777); err != nil {
		return err
	}

	data, err := yaml.Marshal(ruleFile)
	if err != nil {
		return err
	}

	rulePath := filepath.Join(rulesDir, resource.Name+".yml")
	if err := os.WriteFile(rulePath, data, 0644); err != nil {
		return err
	}

	l.Info("Wrote recording rule file", "file", rulePath, "resource", resource.Name)
	err = f.reloadPrometheus(ctx)
	if err != nil {
		return err
	}
	return nil
}

func (f *FileRecordingRules) Delete(ctx context.Context, resource *chantico.DataCenterResource) error {
	l := log.FromContext(ctx)

	volumePath := config.ValidatedEnv.VolumeLocation
	rulePath := filepath.Join(volumePath, prometheusRulesDir, resource.Name+".yml")

	l.Info("Deleting rule file", "file", rulePath)

	if err := deleteRuleFile(resource); err != nil {
		return err
	}
	if err := f.reloadPrometheus(ctx); err != nil {
		return err
	}
	return nil
}

func deleteRuleFile(resource *chantico.DataCenterResource) error {
	volumePath := config.ValidatedEnv.VolumeLocation
	rulePath := filepath.Join(volumePath, prometheusRulesDir, resource.Name+".yml")
	if err := os.Remove(rulePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (f *FileRecordingRules) reloadPrometheus(ctx context.Context) error {
	l := log.FromContext(ctx)
	host := config.ValidatedEnv.PrometheusServiceHost
	port := config.ValidatedEnv.PrometheusServicePort

	sanitizedPrometheusURL := url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(host, port),
		Path:   "/-/reload",
	}
	resp, err := http.Post(sanitizedPrometheusURL.String(), "", nil)
	if err != nil {
		l.Error(err, "Failed to reload Prometheus")
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		l.Info("Prometheus reload returned status", "status", resp.StatusCode)
		return fmt.Errorf("prometheus reload returned status %d", resp.StatusCode)
	}
	l.Info("Prometheus configuration reloaded")
	return nil
}

type KubePrometheusRecordingRules struct {
	Client client.Client
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

func (f *KubePrometheusRecordingRules) Reconcile(ctx context.Context, resource *chantico.DataCenterResource) error {
	ruleFile, err := BuildRuleFile(&FileRecordingRules{Client: f.Client}, ctx, resource)
	if err != nil {
		return err
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

func (f *KubePrometheusRecordingRules) Delete(ctx context.Context, resource *chantico.DataCenterResource) error {

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
