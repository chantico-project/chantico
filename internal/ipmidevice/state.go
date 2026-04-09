package ipmidevice

import (
	"slices"

	chantico "chantico/api/v1alpha1"
)

type State string

const (
	StateInit                      = "Init"
	StateEntryPoint                = "Entry Point"
	StateSucceededIPMIConfigUpdate = "Succeeded IPMI Config Update"
	StatePendingIPMIReload         = "Pending IPMI Config Reload"
	StateDelete                    = "Delete"
	StateRemove                    = "Remove"
	StateFailed                    = "Failed"
	StateEndPoint                  = "End Point"
)

func UpdateState(
	ipmiDevice *chantico.IPMIDevice,
) {
	// Covers the initialization pathological cases
	if ipmiDevice == nil {
		return
	}
	if ipmiDevice.Status.UpdateGeneration == 0 {
		ipmiDevice.Status.UpdateGeneration = 1
	}

	// TODO: Could be nice to find a better option for this
	// Covers finalizer
	if !slices.Contains(ipmiDevice.ObjectMeta.Finalizers, chantico.IPMIUpdateFinalizer) {
		ipmiDevice.Status.State = StateInit
		return
	}

	// Covers lifecycle related changes
	isDeleted := ipmiDevice.ObjectMeta.GetDeletionTimestamp() != nil
	isGenerationUpToDate := ipmiDevice.Status.UpdateGeneration < ipmiDevice.ObjectMeta.Generation

	if isDeleted {
		switch ipmiDevice.Status.State {
		case StateDelete, StateRemove:
			break
		default:
			ipmiDevice.Status.State = StateDelete
		}
	}

	if isGenerationUpToDate && !isDeleted {
		ipmiDevice.Status.State = StateEntryPoint
	}

	// Realize the update
	switch ipmiDevice.Status.State {
	case "", StateInit, StateEntryPoint:
		ipmiDevice.Status.State = StateEntryPoint
		ipmiDevice.Status.UpdateGeneration = ipmiDevice.ObjectMeta.Generation
		return

	case StateSucceededIPMIConfigUpdate, StatePendingIPMIReload:
		return
	case StateEndPoint, StateFailed, StateRemove, StateDelete:
		return
	default:
		ipmiDevice.Status.State = StateFailed
		return
	}
}
