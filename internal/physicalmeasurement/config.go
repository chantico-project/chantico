package physicalmeasurement

import (
	"maps"

	chantico "chantico/api/v1alpha1"
)

// FileSDTarget represents a single target group in Prometheus file_sd_configs format.
// Prometheus watches these JSON files and automatically picks up changes
// without needing a reload or restart.
// See: https://prometheus.io/docs/prometheus/latest/configuration/configuration/#file_sd_config
type FileSDTarget struct {
	Targets []string          `json:"targets"`
	Labels  map[string]string `json:"labels"`
}

func applyDefaultAndAdditionalLabels(physicalMeasurement *chantico.PhysicalMeasurement, labels map[string]string) map[string]string {
	labels["name"] = physicalMeasurement.Name
	labels["instance"] = physicalMeasurement.Spec.Ip
	if physicalMeasurement.Spec.AdditionalLabels != nil {
		// Prefer the default and existing labels over the additional labels
		merged := maps.Clone(physicalMeasurement.Spec.AdditionalLabels)
		maps.Copy(merged, labels)
		return merged
	}
	return labels
}

// CreateTargetLabels returns the scrape target labels for a PhysicalMeasurement,
// including its default and additional labels.
func CreateTargetLabels(physicalMeasurement *chantico.PhysicalMeasurement) map[string]string {
	labels := make(map[string]string)
	if physicalMeasurement.Spec.Type == chantico.PhysicalMeasurementTypePrometheusExporter {
		labels["type"] = "prometheus-exporter"
	} else {
		// Default to SNMP configuration.
		labels = map[string]string{
			"__param_module": physicalMeasurement.Spec.MeasurementDevice,
			"__param_auth":   physicalMeasurement.Spec.MeasurementDevice,
			"job":            physicalMeasurement.Spec.MeasurementDevice,
			"type":           "snmp",
		}
	}
	return applyDefaultAndAdditionalLabels(physicalMeasurement, labels)
}
