package physicalmeasurement

import (
	chantico "chantico/api/v1alpha1"
	config "chantico/internal/configuration"
	ph "chantico/internal/patch"
	sm "chantico/internal/statemachine"
	"context"
	"os"
	"path/filepath"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	log "sigs.k8s.io/controller-runtime/pkg/log"
)

const prometheusTargetsDir = "prometheus/targets"

// ActionMap defines the actions to execute for each state.
// With file_sd_configs, Prometheus automatically watches the target files
// for changes — no explicit reload or config merging is needed.
var StateMachine = sm.Machine[*chantico.PhysicalMeasurement]{
	Actions: map[string][]sm.ActionFunction[*chantico.PhysicalMeasurement]{
		StateInit: {
			{Type: sm.ActionFunctionPure, Pure: sm.InitializeFinalizer[*chantico.PhysicalMeasurement]},
			{Type: sm.ActionFunctionIO, IO: WriteTargetFile},
		},
		StateRunning: {},
		StateDelete: {
			{Type: sm.ActionFunctionIO, IO: DeleteTargetFile},
			{Type: sm.ActionFunctionPure, Pure: sm.RemoveFinalizer[*chantico.PhysicalMeasurement]},
		},
		StateFailed: {},
	},
	FailState: StateFailed,
}

func retrieveMeasurementDevice(ctx context.Context, kubernetesClient client.Client, physicalMeasurement *chantico.PhysicalMeasurement) (*chantico.MeasurementDevice, error) {
	// Lookup the measurement device to determine which target type to use.
	measurementDevice := &chantico.MeasurementDevice{}
	key := client.ObjectKey{
		Namespace: physicalMeasurement.Namespace,
		Name:      physicalMeasurement.Spec.MeasurementDevice,
	}

	if err := kubernetesClient.Get(ctx, key, measurementDevice); err != nil {
		return nil, err
	}

	return measurementDevice, nil
}

func constructTargetDir(measurementDevice chantico.MeasurementDevice) string {
	volumePath := config.ValidatedEnv.VolumeLocation

	targetsDir := filepath.Join(volumePath, prometheusTargetsDir, string(measurementDevice.GetUID()))

	return targetsDir
}

// WriteTargetFile writes a file_sd_configs JSON target file for this PhysicalMeasurement.
// The file is written to prometheus/targets/<name>.json.
// Prometheus automatically detects changes to these files and updates its scrape targets.
func WriteTargetFile(
	ctx context.Context,
	kubernetesClient client.Client,
	physicalMeasurement *chantico.PhysicalMeasurement,
) *sm.ActionResult {
	l := log.FromContext(ctx)

	measurementDevice, err := retrieveMeasurementDevice(ctx, kubernetesClient, physicalMeasurement)
	if err != nil {
		physicalMeasurement.Status.State = StateFailed
		physicalMeasurement.Status.ErrorMessage = err.Error()
		return &sm.ActionResult{PatchType: ph.PatchResourceStatus}
	}

	target := CreateFileSDTarget(physicalMeasurement.Spec.MeasurementDevice, physicalMeasurement.Spec.Ip, physicalMeasurement.Name)

	targetsDir := constructTargetDir(*measurementDevice)

	if err := os.MkdirAll(targetsDir, 0777); err != nil {
		physicalMeasurement.Status.State = StateFailed
		physicalMeasurement.Status.ErrorMessage = err.Error()
		l.Error(err, "Failed to create targets directory")
		return &sm.ActionResult{PatchType: ph.PatchResourceStatus}
	}

	targetPath := filepath.Join(targetsDir, string(physicalMeasurement.GetUID())+".json")
	if err := WriteFileSDTargets(targetPath, []FileSDTarget{target}); err != nil {
		physicalMeasurement.Status.State = StateFailed
		physicalMeasurement.Status.ErrorMessage = err.Error()
		l.Error(err, "Failed to write target file")
		return &sm.ActionResult{PatchType: ph.PatchResourceStatus}
	}

	l.Info("Wrote file_sd target file", "path", targetPath, "device", physicalMeasurement.Spec.MeasurementDevice)
	physicalMeasurement.Status.State = StateRunning
	physicalMeasurement.Status.UpdateGeneration = physicalMeasurement.Generation
	return &sm.ActionResult{PatchType: ph.PatchResourceStatus}
}

// DeleteTargetFile removes the file_sd_configs target file for this PhysicalMeasurement.
// Prometheus will automatically stop scraping the removed targets.
func DeleteTargetFile(
	ctx context.Context,
	kubernetesClient client.Client,
	physicalMeasurement *chantico.PhysicalMeasurement,
) *sm.ActionResult {
	l := log.FromContext(ctx)

	measurementDevice, err := retrieveMeasurementDevice(ctx, kubernetesClient, physicalMeasurement)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// MeasurementDevice (and its targets directory) is already gone, so there is nothing left to clean up.
			l.Info("MeasurementDevice no longer exists, skipping target file deletion.")
			return &sm.ActionResult{PatchType: ph.PatchResourceStatus}
		}
		physicalMeasurement.Status.State = StateFailed
		physicalMeasurement.Status.ErrorMessage = err.Error()
		l.Error(err, "Failed to look up MeasurementDevice when deleting target file.")
		return &sm.ActionResult{PatchType: ph.PatchResourceStatus}
	}

	targetPath := filepath.Join(constructTargetDir(*measurementDevice), string(physicalMeasurement.GetUID())+".json")

	l.Info("Deleting target file")

	err = os.Remove(targetPath)
	if err != nil && !os.IsNotExist(err) {
		physicalMeasurement.Status.State = StateFailed
		physicalMeasurement.Status.ErrorMessage = err.Error()
		l.Error(err, "Failed to delete target file")
		return &sm.ActionResult{PatchType: ph.PatchResourceStatus}
	}

	return &sm.ActionResult{PatchType: ph.PatchResourceStatus}
}
