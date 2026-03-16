package measurementdevice

// import (
// 	"slices"
// 	"time"

// 	chantico "chantico/api/v1alpha1"

// 	batchv1 "k8s.io/api/batch/v1"
// )

// type State string

// const (
// 	StateInit                      = "Init"
// 	StateEntryPoint                = "Entry Point"
// 	StatePendingSNMPConfigUpdate   = "Pending SNMP Config Update"
// 	StateSucceededSNMPConfigUpdate = "Succeeded SNMP Config Update"
// 	StatePendingSNMPReload         = "Pending SNMP Config Reload"
// 	StateDelete                    = "Delete"
// 	StateRemove                    = "Remove"
// 	StateFailed                    = "Failed"
// 	StateEndPoint                  = "End Point"
// )

// func UpdateState(
// 	measurementDevice *chantico.MeasurementDevice,
// 	snmpJob *batchv1.Job,
// ) {
// 	// Covers the initialization pathological cases
// 	if measurementDevice == nil {
// 		return
// 	}
// 	if measurementDevice.Status.UpdateGeneration == 0 {
// 		measurementDevice.Status.UpdateGeneration = 1
// 	}

// 	// TODO: Could be nice to find a better option for this
// 	// Covers finalizer
// 	if !slices.Contains(measurementDevice.ObjectMeta.Finalizers, chantico.SNMPUpdateFinalizer) {
// 		measurementDevice.Status.State = StateInit
// 		return
// 	}

// 	// Covers lifecycle related changes
// 	isDeleted := measurementDevice.ObjectMeta.GetDeletionTimestamp() != nil
// 	isGenerationUpToDate := measurementDevice.Status.UpdateGeneration < measurementDevice.ObjectMeta.Generation // IVO: how does this generation relate to the state?

// 	if isDeleted {
// 		switch measurementDevice.Status.State {
// 		case StateDelete, StateRemove:
// 			break
// 		default:
// 			measurementDevice.Status.State = StateDelete
// 		}
// 	}

// 	if isGenerationUpToDate && !isDeleted {
// 		measurementDevice.Status.State = StateEntryPoint
// 	}

// 	// Realize the update
// 	switch measurementDevice.Status.State {
// 	case "", StateInit, StateEntryPoint:
// 		measurementDevice.Status.State = StateEntryPoint
// 		measurementDevice.Status.UpdateGeneration = measurementDevice.ObjectMeta.Generation
// 		return

// 	case StatePendingSNMPConfigUpdate:
// 		if snmpJob.Status.Succeeded == 1 {
// 			measurementDevice.Status.State = StateSucceededSNMPConfigUpdate
// 		} else if snmpJob.Status.Failed > 0 {
// 			measurementDevice.Status.State = StateFailed
// 			// log.Fatalf("JOB: %#v", snmpJob)
// 		} else {
// 			startTime := snmpJob.Status.StartTime
// 			if startTime == nil {
// 				break
// 			}
// 			now := time.Now()
// 			if startTime.Time.Add(chantico.SNMPJobTimeout).Before(now) {
// 				measurementDevice.Status.State = StateFailed
// 				// log.Fatalf("JOB: %v, %v", startTime.Time, now)
// 			}
// 		}
// 		return
// 	case StateSucceededSNMPConfigUpdate, StatePendingSNMPReload:
// 		return
// 	case StateEndPoint, StateFailed, StateRemove, StateDelete:
// 		return
// 	default:
// 		measurementDevice.Status.State = StateFailed
// 		return
// 	}
// }
