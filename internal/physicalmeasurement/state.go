package physicalmeasurement

import (
	chantico "chantico/api/v1alpha1"
	"slices"
)

type State string

const (
	StateInit               = "init"
	StateRunning            = "Running"
	StateRunningWithWarning = "Running (with warning)"
	StateDelete             = "Delete"
	StateFailed             = "Failed"
)

func UpdateState(
	physicalMeasurement *chantico.PhysicalMeasurement,
) {
	if physicalMeasurement == nil {
		return
	}
	if physicalMeasurement.Status.UpdateGeneration == 0 {
		physicalMeasurement.Status.UpdateGeneration = 1
	}

	if !slices.Contains(physicalMeasurement.ObjectMeta.Finalizers, chantico.PhysicalMeasurementFinalizer) {
		physicalMeasurement.Status.State = StateInit
		return
	}

	// Covers lifecycle related changes
	isDeleted := physicalMeasurement.ObjectMeta.GetDeletionTimestamp() != nil
	needsReconcile := physicalMeasurement.Status.UpdateGeneration < physicalMeasurement.ObjectMeta.Generation

	if isDeleted {
		switch physicalMeasurement.Status.State {
		case StateDelete:
			break
		default:
			physicalMeasurement.Status.State = StateDelete
		}
	}

	if needsReconcile && !isDeleted {
		physicalMeasurement.Status.State = StateInit
	} else if !needsReconcile && !isDeleted {
		switch physicalMeasurement.Status.State {
		case StateRunningWithWarning:
			// Do nothing.
		default:
			physicalMeasurement.Status.State = StateRunning
		}
	}

	switch physicalMeasurement.Status.State {
	case "", StateInit:
		physicalMeasurement.Status.State = StateInit
		return
	case StateRunning, StateRunningWithWarning, StateDelete, StateFailed:
		return
	default:
		physicalMeasurement.Status.State = StateFailed
		return
	}
}
