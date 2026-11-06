package physicalmeasurement

import (
	chantico "chantico/api/v1alpha1"
	"chantico/internal/k8s"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	monitoringv1alpha1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type FileTargets struct{}

type KubePrometheusTargets struct {
	Client              client.Client
	SNMPExporterAddress string
}

func (targets *KubePrometheusTargets) Reconcile(
	ctx context.Context,
	measurement *chantico.PhysicalMeasurement,
) error {
	var metricsPath string
	// TODO: Add this to the PhysicalMeasurementSpec as a configurable field
	if measurement.Spec.Type == chantico.PhysicalMeasurementTypePrometheusExporter {
		metricsPath = "/metrics"
	} else {
		metricsPath = "/snmp"
	}

	scrapeConfig := &monitoringv1alpha1.ScrapeConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      measurement.Name,
			Namespace: measurement.Namespace,
			Labels:    k8s.PrometheusResourceLabels(),
		},
		Spec: monitoringv1alpha1.ScrapeConfigSpec{
			MetricsPath: &metricsPath,
			StaticConfigs: []monitoringv1alpha1.StaticConfig{
				{
					Targets: []monitoringv1alpha1.Target{
						monitoringv1alpha1.Target(measurement.Spec.Ip),
					},
					Labels: CreateTargetLabels(measurement),
				},
			},
		},
	}
	if measurement.Spec.Type == chantico.PhysicalMeasurementTypeSNMP {
		if targets.SNMPExporterAddress == "" {
			return fmt.Errorf("SNMP exporter address is required")
		}
		scrapeConfig.Spec.RelabelConfigs = []monitoringv1.RelabelConfig{
			{
				SourceLabels: []monitoringv1.LabelName{"__address__"},
				TargetLabel:  "__param_target",
			},
			{
				SourceLabels: []monitoringv1.LabelName{"__param_target"},
				TargetLabel:  "instance",
			},
			{
				TargetLabel: "__address__",
				Replacement: &targets.SNMPExporterAddress,
			},
		}
	}
	desired := scrapeConfig.DeepCopy()
	_, err := controllerutil.CreateOrUpdate(ctx, targets.Client, scrapeConfig, func() error {
		if scrapeConfig.Labels == nil {
			scrapeConfig.Labels = make(map[string]string)
		}
		for key, value := range desired.Labels {
			scrapeConfig.Labels[key] = value
		}
		scrapeConfig.Spec = desired.Spec
		return nil
	})
	if err != nil {
		return fmt.Errorf("reconcile scrape config: %w", err)
	}
	return nil
}

func (targets *KubePrometheusTargets) Delete(
	ctx context.Context,
	measurement *chantico.PhysicalMeasurement,
) error {
	scrapeConfig := &monitoringv1alpha1.ScrapeConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      measurement.Name,
			Namespace: measurement.Namespace,
		},
	}
	if err := client.IgnoreNotFound(targets.Client.Delete(ctx, scrapeConfig)); err != nil {
		return fmt.Errorf("delete scrape config: %w", err)
	}
	return nil
}
