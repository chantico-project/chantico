package controller

import (
	chantico "chantico/api/v1alpha1"
	ph "chantico/internal/patch"
	"chantico/internal/steps"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	config "chantico/internal/configuration"
	dcr "chantico/internal/datacenterresource"

	"github.com/go-logr/logr"
	yaml "go.yaml.in/yaml/v2"
	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

	return steps.Run(ctx, dataCenterResource,
		r.reconcileDeletion,
		r.ensureFinalizerIsSet,
	)
}

func (r *DataCenterResourceReconciler) reconcileDeletion(ctx context.Context, dataCenterResource *chantico.DataCenterResource) steps.StepResult {
	l := log.FromContext(ctx)

	volumePath := config.ValidatedEnv.VolumeLocation
	rulePath := filepath.Join(volumePath, prometheusRulesDir, dataCenterResource.Name+".yml")

	l.Info("Deleting rule file", "file", rulePath)

	err := os.Remove(rulePath)
	if err != nil && !os.IsNotExist(err) {
		return steps.Error(err)
	}
	reloadPrometheus(ctx)
	return steps.Stop()
}

func (r *DataCenterResourceReconciler) reconcileRuleFile(ctx context.Context, dataCenterResource *chantico.DataCenterResource) steps.StepResult {
	ruleFile := dcr.BuildRuleFile(dataCenterResource)

	// If there are no rules to write (e.g. root node with no children),
	// clean up any stale rule file and return.
	if ruleFile == nil {
		dcr.DeleteRuleFileFromDisk(dataCenterResource.Name)
		return steps.Stop()
	}

	volumePath := config.ValidatedEnv.VolumeLocation
	rulesDir := filepath.Join(volumePath, prometheusRulesDir)
	if err := os.MkdirAll(rulesDir, 0777); err != nil {
		// log.Printf("Failed to create rules directory: %v", err)
		dcr.SetValidationError(dataCenterResource, err, "")
		return steps.Error(err)
	}

	data, err := yaml.Marshal(ruleFile)
	if err != nil {
		// log.Printf("Failed to marshal rule file: %v", err)
		dcr.SetValidationError(dataCenterResource, err, "")
		return steps.Error(err)
	}

	rulePath := filepath.Join(rulesDir, dataCenterResource.Name+".yml")
	if err := os.WriteFile(rulePath, data, 0644); err != nil {
		// log.Printf("Failed to write rule file: %v", err)
		dcr.SetValidationError(dataCenterResource, err, "")
		return steps.Error(err)
	}

	// log.Printf("Wrote recording rule file %s for resource %s\n", rulePath, dataCenterResource.Name)
	reloadPrometheus(ctx)
	return steps.Stop()
}

func (r *DataCenterResourceReconciler) ensureFinalizerIsSet(ctx context.Context, dataCenterResource *chantico.DataCenterResource) steps.StepResult {
	if util.ContainsFinalizer(dataCenterResource, chantico.DataCenterResourceGraphFinalizer) {
		return steps.Continue()
	}
	util.AddFinalizer(dataCenterResource, chantico.DataCenterResourceGraphFinalizer)
	return steps.Stop()
}

// reloadPrometheus sends a POST to the Prometheus /-/reload endpoint so that
// newly written (or deleted) rule files are picked up.  Requires Prometheus to
// be started with --web.enable-lifecycle.
func reloadPrometheus(ctx context.Context) {
	l := log.FromContext(ctx)
	host := config.ValidatedEnv.PrometheusServiceHost
	port := config.ValidatedEnv.PrometheusServicePort
	url := fmt.Sprintf("http://%s:%s/-/reload", host, port)
	resp, err := http.Post(url, "", nil)
	if err != nil {
		l.Error(err, "Failed to reload Prometheus")
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		l.Info("Prometheus reload returned status", "status", resp.StatusCode)
		return
	}
	l.Info("Prometheus configuration reloaded")
}

func (r *DataCenterResourceReconciler) ValidateDataCenterResource(ctx context.Context, req ctrl.Request) ([]chantico.DataCenterResource, error) {
	l := log.FromContext(ctx)

	dataCenterResource := &chantico.DataCenterResource{}
	_ = r.Get(ctx, req.NamespacedName, dataCenterResource)

	listOptions := []client.ListOption{client.InNamespace(req.NamespacedName.Namespace)}
	dataCenterResources := &chantico.DataCenterResourceList{}
	_ = r.List(ctx, dataCenterResources, listOptions...)

	physicalMeasurements := &chantico.PhysicalMeasurementList{}
	_ = r.List(ctx, physicalMeasurements, listOptions...)

	visited, err, involvedResource := dcr.Validate(dataCenterResource, dataCenterResources.Items, physicalMeasurements.Items)
	if err != nil {
		l.Info("Setting validation error", "error", err)
		dcr.SetValidationError(dataCenterResource, err, involvedResource)
		return visited, err
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
	return visited, nil
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
