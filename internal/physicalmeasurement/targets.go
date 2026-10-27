package physicalmeasurement

import (
	"bytes"
	chantico "chantico/api/v1alpha1"

	config "chantico/internal/configuration"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"sigs.k8s.io/controller-runtime/pkg/log"
)

type PrometheusTarget struct {
	Name            string
	Type            string
	Address         string
	Path            string
	RefreshInterval string
}

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

type KubePrometheusTargets struct{}
