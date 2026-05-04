package physicalmeasurement

import (
	chantico "chantico/api/v1alpha1"
	ph "chantico/internal/patch"
	sm "chantico/internal/statemachine"
	vol "chantico/internal/volumes"
	"context"
	"os"
	"path/filepath"

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
			{Type: sm.ActionFunctionPure, Pure: WriteTargetFile},
		},
		StateRunning: {},
		StateDelete: {
			{Type: sm.ActionFunctionPure, Pure: DeleteTargetFile},
			{Type: sm.ActionFunctionPure, Pure: sm.RemoveFinalizer[*chantico.PhysicalMeasurement]},
		},
		StateFailed: {},
	},
	FailState: StateFailed,
}

// WriteTargetFile writes a file_sd_configs JSON target file for this PhysicalMeasurement.
// The file is written to prometheus/targets/<name>.json.
// Prometheus automatically detects changes to these files and updates its scrape targets.
func WriteTargetFile(
	ctx context.Context,
	physicalMeasurement *chantico.PhysicalMeasurement,
) *sm.ActionResult {
	l := log.FromContext(ctx)

	target := CreateFileSDTarget(physicalMeasurement.Spec.SNMPDevice, physicalMeasurement.Spec.Ip)

	volumePath := os.Getenv(vol.ChanticoVolumeLocationEnv)
	targetsDir := filepath.Join(volumePath, prometheusTargetsDir)
	if err := os.MkdirAll(targetsDir, 0777); err != nil {
		physicalMeasurement.Status.State = StateFailed
		physicalMeasurement.Status.ErrorMessage = err.Error()
		l.Error(err, "Failed to create targets directory")
		return &sm.ActionResult{PatchType: ph.PatchResourceStatus}
	}

	targetPath := filepath.Join(targetsDir, physicalMeasurement.Name+".json")
	if err := WriteFileSDTargets(targetPath, []FileSDTarget{target}); err != nil {
		physicalMeasurement.Status.State = StateFailed
		physicalMeasurement.Status.ErrorMessage = err.Error()
		l.Error(err, "Failed to write target file")
		return &sm.ActionResult{PatchType: ph.PatchResourceStatus}
	}

	l.Info("Wrote file_sd target file", "path", targetPath, "device", physicalMeasurement.Spec.SNMPDevice)
	physicalMeasurement.Status.State = StateRunning
	physicalMeasurement.Status.UpdateGeneration = physicalMeasurement.ObjectMeta.Generation
	return &sm.ActionResult{PatchType: ph.PatchResourceStatus}
}

// DeleteTargetFile removes the file_sd_configs target file for this PhysicalMeasurement.
// Prometheus will automatically stop scraping the removed targets.
func DeleteTargetFile(
	ctx context.Context,
	physicalMeasurement *chantico.PhysicalMeasurement,
) *sm.ActionResult {
	l := log.FromContext(ctx)
	volumePath := os.Getenv(vol.ChanticoVolumeLocationEnv)
	targetPath := filepath.Join(volumePath, prometheusTargetsDir, physicalMeasurement.Name+".json")

	l.Info("Deleting target file")

	err := os.Remove(targetPath)
	if err != nil && !os.IsNotExist(err) {
		physicalMeasurement.Status.State = StateFailed
		physicalMeasurement.Status.ErrorMessage = err.Error()
		l.Error(err, "Failed to delete target file")
		return &sm.ActionResult{PatchType: ph.PatchResourceStatus}
	}

	return &sm.ActionResult{PatchType: ph.PatchResourceStatus}
}
