package physicalmeasurement

import (
	"bytes"
	chantico "chantico/api/v1alpha1"
	"chantico/internal/k8s"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	monitoringv1alpha1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	config "chantico/internal/configuration"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type FileTargets struct{}

const prometheusTargetsDir = "prometheus/targets"

type typeToSubdirMap map[chantico.PhysicalMeasurementType]string

var targetFileSubdirs = typeToSubdirMap{
	chantico.PhysicalMeasurementTypeSNMP:               "snmp",
	chantico.PhysicalMeasurementTypePrometheusExporter: "prometheus-exporter",
}

func getTargetFileSubdir(measurement *chantico.PhysicalMeasurement) (string, error) {
	subdir, ok := targetFileSubdirs[measurement.Spec.Type]
	if !ok {
		return "", fmt.Errorf("unsupported physical measurement type: %s", measurement.Spec.Type)
	}
	return subdir, nil
}

func (targets *FileTargets) Reconcile(
	ctx context.Context,
	measurement *chantico.PhysicalMeasurement,
) error {
	l := log.FromContext(ctx)

	target, err := CreateFileSDTarget(measurement)
	if err != nil {
		return fmt.Errorf("create target: %w", err)
	}

	desired, err := MarshalFileSDTargets([]FileSDTarget{target})
	if err != nil {
		return fmt.Errorf("marshal target: %w", err)
	}

	volumePath := config.ValidatedEnv.VolumeLocation
	subdir, err := getTargetFileSubdir(measurement)
	if err != nil {
		return err
	}
	targetsDir := filepath.Join(volumePath, prometheusTargetsDir, subdir)
	targetPath := filepath.Join(targetsDir, measurement.Name+".json")

	observed, err := os.ReadFile(targetPath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("read target file: %w", err)
	}

	if bytes.Equal(observed, desired) {
		return nil
	}

	if err := os.MkdirAll(targetsDir, 0777); err != nil {
		return fmt.Errorf("create targets directory: %w", err)
	}

	if err := WriteFileSDTargets(targetPath, desired); err != nil {
		return fmt.Errorf("write target file: %w", err)
	}

	l.Info("Wrote file_sd target file", "path", targetPath, "device", measurement.Spec.MeasurementDevice)
	return nil

}

func (targets *FileTargets) Delete(
	ctx context.Context,
	measurement *chantico.PhysicalMeasurement,
) error {
	l := log.FromContext(ctx)

	volumePath := config.ValidatedEnv.VolumeLocation
	subdir, err := getTargetFileSubdir(measurement)
	if err != nil {
		return fmt.Errorf("determine target file subdirectory: %w", err)
	}
	targetPath := filepath.Join(volumePath, prometheusTargetsDir, subdir, measurement.Name+".json")

	l.Info("Deleting target file", "path", targetPath)

	err = os.Remove(targetPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete target file: %w", err)
	}

	return nil
}

type KubePrometheusTargets struct {
	Client              client.Client
	SNMPExporterAddress string
}

func (targets *KubePrometheusTargets) Reconcile(
	ctx context.Context,
	measurement *chantico.PhysicalMeasurement,
) error {
	var metricsPath string
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
