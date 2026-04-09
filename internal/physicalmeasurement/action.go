package physicalmeasurement

import (
	chantico "chantico/api/v1alpha1"
	ph "chantico/internal/patch"
	sm "chantico/internal/statemachine"
	"log"
	"os"
	"path/filepath"

	vol "chantico/internal/volumes"
)

const prometheusTargetsDir = "prometheus/targets"

// ActionMap defines the actions to execute for each state.
// With file_sd_configs, Prometheus automatically watches the target files
// for changes — no explicit reload or config merging is needed.
var StateMachine = sm.Machine[*chantico.PhysicalMeasurement]{
	Actions: map[string][]sm.ActionFunction[*chantico.PhysicalMeasurement]{
		StateInit: {
			{Type: sm.ActionFunctionPure, Pure: sm.InitializeFinalizer[*chantico.PhysicalMeasurement]},
			{Type: sm.ActionFunctionPure, Pure: WriteSNMPTargetFile},
			{Type: sm.ActionFunctionPure, Pure: WriteIPMITargetFile},
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

func EnsureTargetsDirExists(
	physicalMeasurement *chantico.PhysicalMeasurement,
) (string, *sm.ActionResult) {
	volumePath := os.Getenv(vol.ChanticoVolumeLocationEnv)
	targetsDir := filepath.Join(volumePath, prometheusTargetsDir)
	if err := os.MkdirAll(targetsDir, 0777); err != nil {
		physicalMeasurement.Status.State = StateFailed
		physicalMeasurement.Status.ErrorMessage = err.Error()
		log.Printf("Failed to create targets directory: %v", err)
		return targetsDir, &sm.ActionResult{PatchType: ph.PatchResourceStatus}
	}
	return targetsDir, nil
}

// WriteTargetFile writes a file_sd_configs JSON target file for this PhysicalMeasurement.
// The file is written to prometheus/targets/<name>.<measurementdevicetype>.json.
// Prometheus automatically detects changes to these files and updates its scrape targets.
func WriteSNMPTargetFile(
	physicalMeasurement *chantico.PhysicalMeasurement,
) *sm.ActionResult {
	if physicalMeasurement.Spec.MeasurementDevice == "" {
		return nil
	}
	target := CreateFileSDTarget(physicalMeasurement.Spec.MeasurementDevice, physicalMeasurement.Spec.Ip)

	targetsDir, result := EnsureTargetsDirExists(physicalMeasurement)
	if result != nil {
		return result
	}

	targetPath := filepath.Join(targetsDir, physicalMeasurement.Name+".snmp.json")
	if err := WriteFileSDTargets(targetPath, []FileSDTarget{target}); err != nil {
		physicalMeasurement.Status.State = StateFailed
		physicalMeasurement.Status.ErrorMessage = err.Error()
		log.Printf("Failed to write target file: %v", err)
		return &sm.ActionResult{PatchType: ph.PatchResourceStatus}
	}

	log.Printf("Wrote file_sd target file %s for device %s\n", targetPath, physicalMeasurement.Spec.MeasurementDevice)
	physicalMeasurement.Status.State = StateRunning
	return &sm.ActionResult{PatchType: ph.PatchResourceStatus}
}

func WriteIPMITargetFile(
	physicalMeasurement *chantico.PhysicalMeasurement,
) *sm.ActionResult {
	if physicalMeasurement.Spec.IPMIDevice == "" {
		return nil
	}
	target := CreateFileSDTarget(physicalMeasurement.Spec.IPMIDevice, physicalMeasurement.Spec.Ip)

	targetsDir, result := EnsureTargetsDirExists(physicalMeasurement)
	if result != nil {
		return result
	}

	targetPath := filepath.Join(targetsDir, physicalMeasurement.Name+".ipmi.json")
	if err := WriteFileSDTargets(targetPath, []FileSDTarget{target}); err != nil {
		physicalMeasurement.Status.State = StateFailed
		physicalMeasurement.Status.ErrorMessage = err.Error()
		log.Printf("Failed to write target file: %v", err)
		return &sm.ActionResult{PatchType: ph.PatchResourceStatus}
	}

	log.Printf("Wrote file_sd target file %s for device %s\n", targetPath, physicalMeasurement.Spec.MeasurementDevice)
	physicalMeasurement.Status.State = StateRunning
	return &sm.ActionResult{PatchType: ph.PatchResourceStatus}
}

// DeleteTargetFile removes the file_sd_configs target file for this PhysicalMeasurement.
// Prometheus will automatically stop scraping the removed targets.
func DeleteTargetFile(physicalMeasurement *chantico.PhysicalMeasurement) *sm.ActionResult {
	volumePath := os.Getenv(vol.ChanticoVolumeLocationEnv)
	targetPaths := []string{
		filepath.Join(volumePath, prometheusTargetsDir, physicalMeasurement.Name+".ipmi.json"),
		filepath.Join(volumePath, prometheusTargetsDir, physicalMeasurement.Name+".snmp.json"),
	}

	log.Printf("Deleting target file(s) for %s\n", physicalMeasurement.Name)
	for _, targetPath := range targetPaths {
		err := os.Remove(targetPath)
		if err != nil && !os.IsNotExist(err) {
			physicalMeasurement.Status.State = StateFailed
			physicalMeasurement.Status.ErrorMessage = err.Error()
			log.Printf("Failed to delete target file %s: %v", targetPath, err)
			return &sm.ActionResult{PatchType: ph.PatchResourceStatus}
		}
	}

	return &sm.ActionResult{PatchType: ph.PatchResourceStatus}
}
