/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	chantico "chantico/api/v1alpha1"
	config "chantico/internal/configuration"
	"chantico/internal/steps"

	pm "chantico/internal/physicalmeasurement"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	util "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	log "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const prometheusTargetsDir = "prometheus/targets"

// +kubebuilder:rbac:groups=chantico-project.github.io,resources=physicalmeasurements,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=chantico-project.github.io,resources=physicalmeasurements/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=chantico-project.github.io,resources=physicalmeasurements/finalizers,verbs=create;update;patch
// +kubebuilder:rbac:groups=chantico-project.github.io,resources=measurementdevices,verbs=get;list;watch

// PhysicalMeasurementReconciler reconciles a PhysicalMeasurement object
type PhysicalMeasurementReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *PhysicalMeasurementReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&chantico.PhysicalMeasurement{}).
		WithLogConstructor(func(req *reconcile.Request) logr.Logger {
			log := mgr.GetLogger().WithName("PhysicalMeasurementController")
			if req != nil {
				log = log.WithValues("resource", req.Name)
			}
			return log
		}).
		Complete(r)
}

func (r *PhysicalMeasurementReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	l := log.FromContext(ctx)

	physicalMeasurement := &chantico.PhysicalMeasurement{}
	err := r.Get(ctx, req.NamespacedName, physicalMeasurement)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	l = l.WithValues("generation", physicalMeasurement.GetGeneration())
	ctx = log.IntoContext(ctx, l)

	// Patches the changes to the PhysicalMeasurement at the end of reconciliation. This updates the observedGeneration and conditions in the status.
	patcher, err := patch.NewHelper(physicalMeasurement, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}
	defer func() {
		if err := patcher.Patch(ctx, physicalMeasurement, patch.WithStatusObservedGeneration{}); err != nil {
			reterr = errors.Join(reterr, err)
		}
	}()

	physicalMeasurement.UpdateStatusCondition(chantico.ConditionReady, metav1.ConditionUnknown, chantico.ReasonReconciling, "Reconciliation is in progress")
	return steps.Run(ctx, physicalMeasurement,
		r.reconcileDeletion,
		r.ensureFinalizerIsSet,
		r.reconcileValidation,
		r.reconcileTargetFile,
		r.reconcileReady,
	)
}

func (r *PhysicalMeasurementReconciler) reconcileDeletion(ctx context.Context, physicalMeasurement *chantico.PhysicalMeasurement) steps.StepResult {
	if physicalMeasurement.GetDeletionTimestamp() == nil {
		return steps.Continue()
	}

	if !util.ContainsFinalizer(physicalMeasurement, chantico.PhysicalMeasurementFinalizer) {
		return steps.Stop()
	}

	l := log.FromContext(ctx)

	volumePath := config.ValidatedEnv.VolumeLocation
	targetPath := filepath.Join(volumePath, prometheusTargetsDir, physicalMeasurement.Name+".json")

	l.Info("Deleting target file", "path", targetPath)

	err := os.Remove(targetPath)
	if err != nil && !os.IsNotExist(err) {
		return failed(physicalMeasurement, chantico.ConditionApplied, chantico.ReasonCleanupFailed, "Error deleting target file", err)
	}

	util.RemoveFinalizer(physicalMeasurement, chantico.PhysicalMeasurementFinalizer)
	return steps.Stop()
}

func (r *PhysicalMeasurementReconciler) ensureFinalizerIsSet(ctx context.Context, physicalMeasurement *chantico.PhysicalMeasurement) steps.StepResult {
	if util.ContainsFinalizer(physicalMeasurement, chantico.PhysicalMeasurementFinalizer) {
		return steps.Continue()
	}
	util.AddFinalizer(physicalMeasurement, chantico.PhysicalMeasurementFinalizer)
	return steps.Stop()
}

func (r *PhysicalMeasurementReconciler) reconcileValidation(ctx context.Context, physicalMeasurement *chantico.PhysicalMeasurement) steps.StepResult {
	deviceName := physicalMeasurement.Spec.MeasurementDevice
	key := types.NamespacedName{Namespace: physicalMeasurement.Namespace, Name: deviceName}

	if err := r.Get(ctx, key, &chantico.MeasurementDevice{}); err != nil {
		if apierrors.IsNotFound(err) {
			log.FromContext(ctx).Info("Referenced MeasurementDevice does not exist yet", "measurementDevice", deviceName)
			return requeued(physicalMeasurement, chantico.ConditionValidated, chantico.ReasonDependencyUnavailable, "MeasurementDevice "+deviceName+" does not exist", chantico.EndpointRequeueDelay)
		}
		return failed(physicalMeasurement, chantico.ConditionValidated, chantico.ReasonInvalidSpec, "Error getting MeasurementDevice "+deviceName, err)
	}

	return reconciled(physicalMeasurement, chantico.ConditionValidated, "Validation successful")
}

func (r *PhysicalMeasurementReconciler) reconcileTargetFile(ctx context.Context, physicalMeasurement *chantico.PhysicalMeasurement) steps.StepResult {
	l := log.FromContext(ctx)

	target := pm.CreateFileSDTarget(physicalMeasurement.Spec.MeasurementDevice, physicalMeasurement.Spec.Ip, physicalMeasurement.Name)
	desired, err := pm.MarshalFileSDTargets([]pm.FileSDTarget{target})
	if err != nil {
		return failed(physicalMeasurement, chantico.ConditionApplied, chantico.ReasonApplyFailed, "Error marshalling target file", err)
	}

	volumePath := config.ValidatedEnv.VolumeLocation
	targetsDir := filepath.Join(volumePath, prometheusTargetsDir)
	targetPath := filepath.Join(targetsDir, physicalMeasurement.Name+".json")

	observed, err := os.ReadFile(targetPath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return failed(physicalMeasurement, chantico.ConditionApplied, chantico.ReasonApplyFailed, "Error reading target file", err)
	}

	if !bytes.Equal(observed, desired) {
		return writeTargetFile(physicalMeasurement, targetsDir, targetPath, desired, l)
	}

	return reconciled(physicalMeasurement, chantico.ConditionApplied, "Target file is up to date")
}

func writeTargetFile(physicalMeasurement *chantico.PhysicalMeasurement, targetsDir, targetPath string, desired []byte, l logr.Logger) steps.StepResult {
	if err := os.MkdirAll(targetsDir, 0777); err != nil {
		return failed(physicalMeasurement, chantico.ConditionApplied, chantico.ReasonApplyFailed, "Error creating targets directory", err)
	}

	if err := pm.WriteFileSDTargets(targetPath, desired); err != nil {
		return failed(physicalMeasurement, chantico.ConditionApplied, chantico.ReasonApplyFailed, "Error writing target file", err)
	}

	l.Info("Wrote file_sd target file", "path", targetPath, "device", physicalMeasurement.Spec.MeasurementDevice)
	return reconciled(physicalMeasurement, chantico.ConditionApplied, "Target file has been generated successfully")
}

func (r *PhysicalMeasurementReconciler) reconcileReady(ctx context.Context, physicalMeasurement *chantico.PhysicalMeasurement) steps.StepResult {
	return reconciled(physicalMeasurement, chantico.ConditionReady, "Reconciliation completed successfully")
}
