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

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	types "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	log "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	chantico "chantico/api/v1alpha1"
	dcr "chantico/internal/datacenterresource"
	ph "chantico/internal/patch"
)

// DataCenterResourceReconciler reconciles a DataCenterResource object
type DataCenterResourceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=chantico.ci.tno.nl,resources=datacenterresources,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=chantico.ci.tno.nl,resources=datacenterresources/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=chantico.ci.tno.nl,resources=datacenterresources/finalizers,verbs=update
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;patch;update;delete
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.22.4/pkg/reconcile
func (r *DataCenterResourceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	dataCenterResource := &chantico.DataCenterResource{}
	_ = r.Get(ctx, req.NamespacedName, dataCenterResource)

	listOptions := []client.ListOption{client.InNamespace(req.NamespacedName.Namespace)}
	dataCenterResources := &chantico.DataCenterResourceList{}
	_ = r.List(ctx, dataCenterResources, listOptions...)

	physicalMeasurements := &chantico.PhysicalMeasurementList{}
	_ = r.List(ctx, physicalMeasurements, listOptions...)

	patch := ph.Initialize(ctx, r.Client, dataCenterResource)

	dcr.UpdateState(dataCenterResource)
	patch.PatchStatus()

	result := dcr.StateMachine.ExecuteActions(ctx, r.Client, dataCenterResource, patch)
	if result != nil && result.Result != nil && (result.Requeue || result.RequeueAfter > 0) {
		return *result.Result, nil
	}

	// Perform validation and clear other visited node validation errors if needed
	// This brings those into a reconciliation loop as well
	visited, err, involvedResource := dcr.Validate(dataCenterResource, dataCenterResources.Items, physicalMeasurements.Items)
	if err != nil {
		l.Info("Setting validation error", "error", err)
		dcr.SetValidationError(dataCenterResource, err, involvedResource)
	} else {
		l.Info("Clearing validation errors")
		l.Info("Previous status", "status", dataCenterResource.Status)

		references := &chantico.DataCenterResourceList{}
		_ = r.List(ctx, references, append(listOptions, client.MatchingFields{"status.involvedResource": dataCenterResource.Name})...)
		children := &chantico.DataCenterResourceList{}
		_ = r.List(ctx, children, append(listOptions, client.MatchingFields{"spec.parents": dataCenterResource.Name})...)
		if dataCenterResource.Status.InvolvedResource != "" {
			involved := &chantico.DataCenterResource{}
			_ = r.Get(ctx, types.NamespacedName{Namespace: req.NamespacedName.Namespace, Name: dataCenterResource.Status.InvolvedResource}, involved)
			visited = append(visited, *involved)
		}
		l.Info("Visited nodes", "nodes", dcr.FormatResources(visited))
		l.Info("Referencing resources", "resources", dcr.FormatResources(references.Items))
		l.Info("Children", "children", dcr.FormatResources(children.Items))
		items := MergeUnique(visited, references.Items, children.Items)

		for _, item := range items {
			r.ClearReferencedValidation(ctx, req, dataCenterResource, &item)
		}
		dcr.ClearValidationError(dataCenterResource)
		dataCenterResource.Status.State = dcr.StateEntry
	}
	patch.PatchStatus()

	// TODO(user): do something with the links here:
	// perform operations to make the cluster state reflect the state specified by
	// the user.
	// Specifically: register in relational/graph db (or prometheus?) which datacenter resource
	// is involved for which physical measurement

	return ctrl.Result{}, nil
}

func MergeUnique(
	lists ...[]chantico.DataCenterResource,
) []chantico.DataCenterResource {
	seen := make(map[string]chantico.DataCenterResource)

	for _, list := range lists {
		for _, item := range list {
			seen[item.Name] = item
		}
	}

	result := make([]chantico.DataCenterResource, 0, len(seen))
	for _, v := range seen {
		result = append(result, v)
	}
	return result
}

func (r *DataCenterResourceReconciler) ClearReferencedValidation(
	ctx context.Context,
	req ctrl.Request,
	dataCenterResource *chantico.DataCenterResource,
	referenced *chantico.DataCenterResource,
) {
	// Revalidate if previously failed or current item is being removed
	if referenced.Status.State == dcr.StateValidationFailed || dataCenterResource.Status.State == dcr.StateDelete {
		patch := ph.Initialize(ctx, r.Client, referenced)
		dcr.ClearValidationError(referenced)
		patch.PatchStatus()
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *DataCenterResourceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	ctx := context.Background()

	// Create a one-to-many index for parent field
	err := mgr.GetFieldIndexer().IndexField(
		ctx,
		&chantico.DataCenterResource{},
		"spec.parents",
		func(rawObj client.Object) []string {
			dcr := rawObj.(*chantico.DataCenterResource)

			if dcr.Spec.Parents == nil {
				return nil
			}
			return dcr.Spec.ParentNames()
		},
	)
	if err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&chantico.DataCenterResource{}).
		Named("datacenterresource").
		WithLogConstructor(func(req *reconcile.Request) logr.Logger {
			log := mgr.GetLogger().WithName("DataCenterResourceController")
			if req != nil {
				log = log.WithValues("resource", req.Name)
			}
			return log
		}).
		Complete(r)
}
