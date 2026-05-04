package physicalmeasurement

import (
	"encoding/json"
	"os"
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
func CreateFileSDTarget(deviceId string, ip string) FileSDTarget {
	return FileSDTarget{
		Targets: []string{ip},
		Labels: map[string]string{
			"__param_module": deviceId,
			"__param_auth":   deviceId,
			"job":            deviceId,
		},
	}
}

// WriteFileSDTargets marshals the targets to JSON and writes them to the given path.
func WriteFileSDTargets(path string, targets []FileSDTarget) error {
	data, err := json.MarshalIndent(targets, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// LoadFileSDTargets reads and parses a file_sd_configs JSON file.
func LoadFileSDTargets(path string) ([]FileSDTarget, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var targets []FileSDTarget
	if err := json.Unmarshal(data, &targets); err != nil {
		return nil, err
	}

	return targets, nil
}
