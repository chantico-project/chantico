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
	"context"
	"errors"

	chantico "chantico/api/v1alpha1"
	"chantico/internal/steps"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	util "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	log "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// +kubebuilder:rbac:groups=chantico-project.github.io,resources=physicalmeasurements,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=chantico-project.github.io,resources=physicalmeasurements/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=chantico-project.github.io,resources=physicalmeasurements/finalizers,verbs=create;update;patch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;patch;update;delete
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;update;patch

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
		return ctrl.Result{}, nil
	}
	l = l.WithValues("generation", physicalMeasurement.GetGeneration())
	ctx = log.IntoContext(ctx, l)

	// Patches the changes to the MeasurementDevice at the end of reconciliation. This updates the observedGeneration and conditions in the status.
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

func (r *PhysicalMeasurementReconciler) reconcileTargetFile(ctx context.Context, physicalMeasurement *chantico.PhysicalMeasurement) steps.StepResult {
	return steps.Stop()
}

func (r *PhysicalMeasurementReconciler) reconcileReady(ctx context.Context, physicalMeasurement *chantico.PhysicalMeasurement) steps.StepResult {
	physicalMeasurement.UpdateStatusCondition(chantico.ConditionReady, metav1.ConditionTrue, chantico.ReasonReconciled, "Reconciliation completed successfully")
	return steps.Continue()
}
