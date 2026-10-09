package physicalmeasurement

import (
	"bytes"
	"chantico/internal/filestore"
	"context"
	"encoding/json"
	"fmt"

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

// CreateFileSDTarget creates a file_sd_configs target entry for a PhysicalMeasurement.
// The labels __param_module and __param_auth are used by the SNMP exporter relabel
// configs in prometheus.yml to route scrapes through the correct SNMP module.
func CreateFileSDTarget(physicalMeasurement *chantico.PhysicalMeasurement) (FileSDTarget, error) {
	switch physicalMeasurement.Spec.Type {
	case chantico.PhysicalMeasurementTypePrometheusExporter:
		return FileSDTarget{
			Targets: []string{physicalMeasurement.Spec.Ip},
			Labels: map[string]string{
				"name":     physicalMeasurement.Name,
				"instance": physicalMeasurement.Spec.Ip,
				"type":     "prometheus-exporter",
			},
		}, nil
	case chantico.PhysicalMeasurementTypeSNMP:
		return FileSDTarget{
			Targets: []string{physicalMeasurement.Spec.Ip},
			Labels: map[string]string{
				"__param_module": physicalMeasurement.Spec.MeasurementDevice,
				"__param_auth":   physicalMeasurement.Spec.MeasurementDevice,
				"job":            physicalMeasurement.Spec.MeasurementDevice,
				"name":           physicalMeasurement.Name,
				"instance":       physicalMeasurement.Spec.Ip,
				"type":           "snmp",
			},
		}, nil
	}
	return FileSDTarget{}, fmt.Errorf("unsupported physical measurement type: %q", physicalMeasurement.Spec.Type)
}

// MarshalFileSDTargets renders the targets as the JSON content of a file_sd_configs file.
func MarshalFileSDTargets(targets []FileSDTarget) ([]byte, error) {
	return json.MarshalIndent(targets, "", "  ")
}

// WriteFileSDTargets writes the file_sd_configs JSON atomically, so Prometheus never
// reads a partially written target file.
func WriteFileSDTargets(path string, data []byte) error {
	vfs := filestore.VolumeFileStore{}
	return vfs.Write(context.Background(), path, bytes.NewReader(data))
}

// LoadFileSDTargets reads and parses a file_sd_configs JSON file.
func LoadFileSDTargets(filestore filestore.FileStore, path string) ([]FileSDTarget, error) {
	data, err := filestore.ReadAll(context.Background(), path)
	if err != nil {
		return nil, err
	}

	var targets []FileSDTarget
	if err := json.Unmarshal(data, &targets); err != nil {
		return nil, err
	}

	return targets, nil
}
