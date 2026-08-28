package controller

import (
	chantico "chantico/api/v1alpha1"
	ph "chantico/internal/patch"
	"chantico/internal/steps"
	"context"
	"errors"
	"os"
	"path/filepath"

	config "chantico/internal/configuration"
	dcr "chantico/internal/datacenterresource"

	"github.com/go-logr/logr"
	yaml "go.yaml.in/yaml/v2"
	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlcontroller "sigs.k8s.io/controller-runtime/pkg/controller"
	util "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	log "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const prometheusRulesDir = "prometheus/rules"

// +kubebuilder:rbac:groups=chantico-project.github.io,resources=datacenterresources,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=chantico-project.github.io,resources=datacenterresources/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=chantico-project.github.io,resources=datacenterresources/finalizers,verbs=update
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;patch;update;delete
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;update;patch

// DataCenterResourceReconciler reconciles a DataCenterResource object
type DataCenterResourceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *DataCenterResourceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&chantico.DataCenterResource{}).
		Owns(&batchv1.Job{}).
		WithOptions(ctrlcontroller.Options{MaxConcurrentReconciles: 1}). // Race conditions might occur when multiple generator jobs run simultaneously, so only allow one at a time.
		WithLogConstructor(func(req *reconcile.Request) logr.Logger {
			log := mgr.GetLogger().WithName("DataCenterResourceController")
			if req != nil {
				log = log.WithValues("resource", req.Name)
			}
			return log
		}).
		Complete(r)
}

func (r *DataCenterResourceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	l := log.FromContext(ctx)

	dataCenterResource := &chantico.DataCenterResource{}
	err := r.Get(ctx, req.NamespacedName, dataCenterResource)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	l = l.WithValues("generation", dataCenterResource.GetGeneration())
	ctx = log.IntoContext(ctx, l)

	// Patches the changes to the DataCenterResource at the end of reconciliation. This updates the observedGeneration and conditions in the status.
	patcher, err := patch.NewHelper(dataCenterResource, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}
	defer func() {
		if err := patcher.Patch(ctx, dataCenterResource, patch.WithStatusObservedGeneration{}); err != nil {
			reterr = errors.Join(reterr, err)
		}
	}()

	dataCenterResource.UpdateStatusCondition(chantico.ConditionReady, metav1.ConditionUnknown, chantico.ReasonReconciling, "Reconciliation is in progress")
	return steps.Run(ctx, dataCenterResource,
		r.reconcileDeletion,
		r.ensureFinalizerIsSet,
		r.reconcileValidation,
		r.reconcileWriteRuleFile,
		r.reconcileReady,
	)
}

func (r *DataCenterResourceReconciler) reconcileDeletion(ctx context.Context, dataCenterResource *chantico.DataCenterResource) steps.StepResult {
	if dataCenterResource.DeletionTimestamp == nil {
		return steps.Continue()
	}

	if !util.ContainsFinalizer(dataCenterResource, chantico.DataCenterResourceGraphFinalizer) {
		return steps.Stop()
	}

	l := log.FromContext(ctx)

	volumePath := config.ValidatedEnv.VolumeLocation
	rulePath := filepath.Join(volumePath, prometheusRulesDir, dataCenterResource.Name+".yml")

	l.Info("Deleting rule file", "file", rulePath)

	err := os.Remove(rulePath)
	if err != nil && !os.IsNotExist(err) {
		dataCenterResource.UpdateStatusCondition(chantico.ConditionApplied, metav1.ConditionFalse, chantico.ReasonCleanupFailed, "Error deleting rule file: "+err.Error())
		return steps.Error(err)
	}
	err = dcr.ReloadPrometheus(ctx)
	if err != nil {
		dataCenterResource.UpdateStatusCondition(chantico.ConditionApplied, metav1.ConditionFalse, chantico.ReasonReloadFailed, "Error reloading Prometheus: "+err.Error())
		return steps.Error(err)
	}

	util.RemoveFinalizer(dataCenterResource, chantico.DataCenterResourceGraphFinalizer)
	return steps.Stop()
}

func (r *DataCenterResourceReconciler) ensureFinalizerIsSet(ctx context.Context, dataCenterResource *chantico.DataCenterResource) steps.StepResult {
	if util.ContainsFinalizer(dataCenterResource, chantico.DataCenterResourceGraphFinalizer) {
		return steps.Continue()
	}
	util.AddFinalizer(dataCenterResource, chantico.DataCenterResourceGraphFinalizer)
	return steps.Stop()
}

func (r *DataCenterResourceReconciler) reconcileValidation(ctx context.Context, dataCenterResource *chantico.DataCenterResource) steps.StepResult {
	l := log.FromContext(ctx)

	listOptions := []client.ListOption{client.InNamespace(dataCenterResource.Namespace)}
	dataCenterResources := &chantico.DataCenterResourceList{}
	_ = r.List(ctx, dataCenterResources, listOptions...)

	physicalMeasurements := &chantico.PhysicalMeasurementList{}
	_ = r.List(ctx, physicalMeasurements, listOptions...)

	visited, involvedResource, err := dcr.Validate(dataCenterResource, dataCenterResources.Items, physicalMeasurements.Items)
	if err != nil {
		l.Info("Setting validation error", "error", err)
		dataCenterResource.Status.InvolvedResource = involvedResource
		dataCenterResource.UpdateStatusCondition(chantico.ConditionValidated, metav1.ConditionFalse, validationFailureReason(err), err.Error())
		return steps.Error(err)
	} else {
		l.Info("Clearing validation errors")
		l.Info("Previous status", "status", dataCenterResource.Status)

		references := &chantico.DataCenterResourceList{}
		_ = r.List(ctx, references, append(listOptions, client.MatchingFields{"status.involvedResource": dataCenterResource.Name})...)
		children := &chantico.DataCenterResourceList{}
		_ = r.List(ctx, children, append(listOptions, client.MatchingFields{"spec.parents": dataCenterResource.Name})...)
		if dataCenterResource.Status.InvolvedResource != "" {
			involved := &chantico.DataCenterResource{}
			_ = r.Get(ctx, types.NamespacedName{Namespace: dataCenterResource.Namespace, Name: dataCenterResource.Status.InvolvedResource}, involved)
			visited = append(visited, *involved)
		}
		l.Info("Visited nodes", "nodes", dcr.FormatResources(visited))
		l.Info("Referencing resources", "resources", dcr.FormatResources(references.Items))
		l.Info("Children", "children", dcr.FormatResources(children.Items))
		items := mergeUnique(visited, references.Items, children.Items)

		for _, item := range items {
			r.clearReferencedValidation(ctx, dataCenterResource, &item)
		}
		dataCenterResource.Status.InvolvedResource = ""
		dataCenterResource.UpdateStatusCondition(chantico.ConditionValidated, metav1.ConditionTrue, chantico.ReasonReconciled, "Validation successful")
	}
	return steps.Continue()
}

func (r *DataCenterResourceReconciler) reconcileWriteRuleFile(ctx context.Context, dataCenterResource *chantico.DataCenterResource) steps.StepResult {
	l := log.FromContext(ctx)
	ruleFile := dcr.BuildRuleFile(dataCenterResource)

	if ruleFile == nil {
		l.Info("No rule file found")
		dcr.DeleteRuleFileFromDisk(dataCenterResource.Name)
		return steps.Stop()
	}

	volumePath := config.ValidatedEnv.VolumeLocation
	rulesDir := filepath.Join(volumePath, prometheusRulesDir)
	if err := os.MkdirAll(rulesDir, 0777); err != nil {
		dataCenterResource.UpdateStatusCondition(chantico.ConditionApplied, metav1.ConditionFalse, chantico.ReasonApplyFailed, "Failed to create directory "+rulesDir+": "+err.Error())
		return steps.Error(err)
	}

	data, err := yaml.Marshal(ruleFile)
	if err != nil {
		dataCenterResource.UpdateStatusCondition(chantico.ConditionApplied, metav1.ConditionFalse, chantico.ReasonApplyFailed, "Failed to marshal rule file: "+err.Error())
		return steps.Error(err)
	}

	rulePath := filepath.Join(rulesDir, dataCenterResource.Name+".yml")
	if err := os.WriteFile(rulePath, data, 0644); err != nil {
		dataCenterResource.UpdateStatusCondition(chantico.ConditionApplied, metav1.ConditionFalse, chantico.ReasonApplyFailed, "Failed to write rule file: "+err.Error())
		return steps.Error(err)
	}

	l.Info("Wrote recording rule file", "file", rulePath, "resource", dataCenterResource.Name)
	err = dcr.ReloadPrometheus(ctx)
	if err != nil {
		dataCenterResource.UpdateStatusCondition(chantico.ConditionApplied, metav1.ConditionFalse, chantico.ReasonReloadFailed, "Failed to reload Prometheus: "+err.Error())
		return steps.Error(err)
	}

	dataCenterResource.UpdateStatusCondition(chantico.ConditionApplied, metav1.ConditionTrue, chantico.ReasonReconciled, "Recording rule file applied successfully")
	return steps.Continue()
}

func (r *DataCenterResourceReconciler) reconcileReady(ctx context.Context, dataCenterResource *chantico.DataCenterResource) steps.StepResult {
	dataCenterResource.UpdateStatusCondition(chantico.ConditionReady, metav1.ConditionTrue, chantico.ReasonReconciled, "Fully reconciled and ready")
	return steps.Continue()
}

func validationFailureReason(err error) chantico.ConditionReason {
	var missingResource dcr.ErrorResourceNotFound
	if errors.As(err, &missingResource) {
		return chantico.ReasonDependencyUnavailable
	}
	return chantico.ReasonInvalidSpec
}

func mergeUnique(
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

func (r *DataCenterResourceReconciler) clearReferencedValidation(
	ctx context.Context,
	dataCenterResource *chantico.DataCenterResource,
	referenced *chantico.DataCenterResource,
) {
	referenced.GetConditions()
	// Revalidate if previously failed or current item is being removed
	if meta.IsStatusConditionFalse(*referenced.GetConditions(), string(chantico.ConditionValidated)) || meta.IsStatusConditionFalse(*dataCenterResource.GetConditions(), string(chantico.ConditionValidated)) {
		patch := ph.Initialize(ctx, r.Client, referenced)
		referenced.Status.InvolvedResource = ""
		patch.PatchStatus()
	}
}
