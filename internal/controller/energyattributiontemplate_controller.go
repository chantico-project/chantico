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

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"

	md "chantico/internal/measurementdevice"
	"chantico/internal/steps"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlcontroller "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	log "sigs.k8s.io/controller-runtime/pkg/log"
)

// +kubebuilder:rbac:groups=chantico-project.github.io,resources=energyattributiontemplate,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=chantico-project.github.io,resources=energyattributiontemplate/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=chantico-project.github.io,resources=energyattributiontemplate/finalizers,verbs=create;update;patch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;patch;update;delete
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;update;patch

// EnergyAttributionTemplateReconciler reconciles a MeasurementDevice
type EnergyAttributionTemplateReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Paths     md.Paths
	Namespace string
}

func (r *EnergyAttributionTemplateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&chantico.EnergyAttributionTemplate{}).
		Owns(&batchv1.Job{}).
		WithOptions(ctrlcontroller.Options{MaxConcurrentReconciles: 1}). // Race conditions might occur when multiple generator jobs run simultaneously, so only allow one at a time.
		WithLogConstructor(func(req *reconcile.Request) logr.Logger {
			log := mgr.GetLogger().WithName("EnergyAttributionTemplateController")
			if req != nil {
				log = log.WithValues("resource", req.Name)
			}
			return log
		}).
		Complete(r)
}

func (r *EnergyAttributionTemplateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	l := log.FromContext(ctx)

	attributionTemplate := &chantico.EnergyAttributionTemplate{}
	err := r.Get(ctx, req.NamespacedName, attributionTemplate)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	r.Namespace = attributionTemplate.GetNamespace()
	l = l.WithValues("generation", attributionTemplate.GetGeneration())
	ctx = log.IntoContext(ctx, l)

	// Patches the changes to the EnergyAttributionTemplate at the end of reconciliation. This updates the observedGeneration and conditions in the status.
	patcher, err := patch.NewHelper(attributionTemplate, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}
	defer func() {
		if err := patcher.Patch(ctx, attributionTemplate, patch.WithStatusObservedGeneration{}); err != nil {
			reterr = errors.Join(reterr, err)
		}
	}()

	attributionTemplate.UpdateStatusCondition(chantico.ConditionReady, metav1.ConditionUnknown, chantico.ReasonReconciling, "Reconciliation is in progress")
	return steps.Run(ctx, attributionTemplate,
		r.reconcileReady,
	)

}

func (r *EnergyAttributionTemplateReconciler) reconcileReady(ctx context.Context, attributionTemplate *chantico.EnergyAttributionTemplate) steps.StepResult {
	attributionTemplate.UpdateStatusCondition(chantico.ConditionReady, metav1.ConditionTrue, chantico.ReasonReconciled, "Fully reconciled and ready")
	return steps.Continue()
}
