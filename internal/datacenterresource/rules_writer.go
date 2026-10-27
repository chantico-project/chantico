package datacenterresource

import (
	chantico "chantico/api/v1alpha1"
	config "chantico/internal/configuration"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type FileWriter struct {
	Client client.Client
}

const prometheusRulesDir = "prometheus/rules"

func (f *FileWriter) Reconcile(ctx context.Context, resource *chantico.DataCenterResource) error {
	l := log.FromContext(ctx)

	renderer := NewTemplateRenderer(f.Client, resource.Namespace)
	resolvedExpressions, err := renderer.ResolvedExpressions(ctx, resource)
	if err != nil {
		return err
	}

	ruleFile := NewRuleBuilder(resource, resolvedExpressions).BuildRuleFile()

	if ruleFile == nil {
		l.Info("No rule file found")
		if err := deleteRuleFile(resource); err != nil {
			return err
		}
		if err := f.reloadPrometheus(ctx); err != nil {
			return err
		}
		resource.UpdateStatusCondition(chantico.ConditionApplied, metav1.ConditionTrue, chantico.ReasonReconciled, "No recording rule file required")
		return nil
	}

	volumePath := config.ValidatedEnv.VolumeLocation
	rulesDir := filepath.Join(volumePath, prometheusRulesDir)
	if err := os.MkdirAll(rulesDir, 0777); err != nil {
		resource.UpdateStatusCondition(chantico.ConditionApplied, metav1.ConditionFalse, chantico.ReasonApplyFailed, "Failed to create directory "+rulesDir+": "+err.Error())
		return err
	}

	data, err := yaml.Marshal(ruleFile)
	if err != nil {
		resource.UpdateStatusCondition(chantico.ConditionApplied, metav1.ConditionFalse, chantico.ReasonApplyFailed, "Failed to marshal rule file: "+err.Error())
		return err
	}

	rulePath := filepath.Join(rulesDir, resource.Name+".yml")
	if err := os.WriteFile(rulePath, data, 0644); err != nil {
		resource.UpdateStatusCondition(chantico.ConditionApplied, metav1.ConditionFalse, chantico.ReasonApplyFailed, "Failed to write rule file: "+err.Error())
		return err
	}

	l.Info("Wrote recording rule file", "file", rulePath, "resource", resource.Name)
	err = f.reloadPrometheus(ctx)
	if err != nil {
		resource.UpdateStatusCondition(chantico.ConditionApplied, metav1.ConditionFalse, chantico.ReasonReloadFailed, "Failed to reload Prometheus: "+err.Error())
		return err
	}
	return nil
}

func (f *FileWriter) Delete(ctx context.Context, resource *chantico.DataCenterResource) error {
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

func (f *FileWriter) reloadPrometheus(ctx context.Context) error {
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

type KubePrometheusWriter struct {
	Client client.Client
}

func (f *KubePrometheusWriter) Reconcile(ctx context.Context, resource *chantico.DataCenterResource) error {
	// Implement the reconciliation logic for the kube prometheus writer here.

	// rule := &monitoringv1.PrometheusRule{

	// }

	return nil
}

func (f *KubePrometheusWriter) Delete(ctx context.Context, resource *chantico.DataCenterResource) error {
	// Implement the deletion logic for the kube prometheus writer here.
	return nil
}
