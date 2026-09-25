package controller

import (
	"bytes"
	chantico "chantico/api/v1alpha1"
	ph "chantico/internal/patch"
	"chantico/internal/steps"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"text/template"

	config "chantico/internal/configuration"
	dcr "chantico/internal/datacenterresource"

	"github.com/go-logr/logr"
	yaml "go.yaml.in/yaml/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
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
	"sigs.k8s.io/controller-runtime/pkg/handler"
	log "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const prometheusRulesDir = "prometheus/rules"

// +kubebuilder:rbac:groups=chantico-project.github.io,resources=datacenterresources,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=chantico-project.github.io,resources=datacenterresources/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=chantico-project.github.io,resources=datacenterresources/finalizers,verbs=update
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;patch;update;delete
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list

// DataCenterResourceReconciler reconciles a DataCenterResource object
type DataCenterResourceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *DataCenterResourceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&chantico.DataCenterResource{}).
		Owns(&batchv1.Job{}).
		// Watch config maps for updates. Only trigger reconciliation if the configmap is referenced in either coefficient or energyMetric template
		Watches(&corev1.ConfigMap{}, handler.EnqueueRequestsFromMapFunc(r.dataCenterResourcesForConfigMap)).
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

func (r *DataCenterResourceReconciler) dataCenterResourcesForConfigMap(ctx context.Context, configMap client.Object) []reconcile.Request {
	dataCenterResources := &chantico.DataCenterResourceList{}
	if err := r.List(ctx, dataCenterResources, client.InNamespace(configMap.GetNamespace())); err != nil {
		log.FromContext(ctx).Error(err, "Failed to list DataCenterResources for ConfigMap", "configMap", configMap.GetName(), "namespace", configMap.GetNamespace())
		return nil
	}

	requests := make([]reconcile.Request, 0, len(dataCenterResources.Items))
	for _, dataCenterResource := range dataCenterResources.Items {
		if !dataCenterResourceReferencesConfigMap(&dataCenterResource, configMap.GetName()) {
			continue
		}
		requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{
			Namespace: dataCenterResource.Namespace,
			Name:      dataCenterResource.Name,
		}})
	}
	return requests
}

// Check if either energyMetricFrom or parent.CoefficientFrom references this configmap.
func dataCenterResourceReferencesConfigMap(dataCenterResource *chantico.DataCenterResource, configMapName string) bool {
	if dataCenterResource.Spec.EnergyMetricFrom.ConfigMapKeyRef.Name == configMapName {
		return true
	}
	for _, parent := range dataCenterResource.Spec.Parents {
		if parent.CoefficientFrom.ConfigMapKeyRef.Name == configMapName {
			return true
		}
	}
	return false
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

	if err := deleteRuleFile(dataCenterResource); err != nil {
		dataCenterResource.UpdateStatusCondition(chantico.ConditionApplied, metav1.ConditionFalse, chantico.ReasonCleanupFailed, "Error deleting rule file: "+err.Error())
		return steps.Error(err)
	}
	if err := reloadPrometheus(ctx); err != nil {
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
		l.Info("Clearing validation errors", "status", dataCenterResource.Status)
		references := &chantico.DataCenterResourceList{}
		_ = r.List(ctx, references, append(listOptions, client.MatchingFields{"status.involvedResource": dataCenterResource.Name})...)
		children := &chantico.DataCenterResourceList{}
		_ = r.List(ctx, children, append(listOptions, client.MatchingFields{"spec.parents": dataCenterResource.Name})...)
		if dataCenterResource.Status.InvolvedResource != "" {
			involved := &chantico.DataCenterResource{}
			_ = r.Get(ctx, types.NamespacedName{Namespace: dataCenterResource.Namespace, Name: dataCenterResource.Status.InvolvedResource}, involved)
			visited = append(visited, *involved)
		}
		l.Info("Visited nodes", "nodes", dcr.FormatResources(visited), "references", dcr.FormatResources(references.Items), "children", dcr.FormatResources(children.Items))
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

	// Resolve the coefficient templates and apply them to the data center resource
	resolvedDataCenterResource := dataCenterResource.DeepCopy()
	resolvedDataCenterResource, err := r.resolveCoefficientTemplates(ctx, resolvedDataCenterResource)
	if err != nil {
		dataCenterResource.UpdateStatusCondition(chantico.ConditionApplied, metav1.ConditionFalse, chantico.ReasonTemplateResolutionFailed, "Failed to resolve coefficient template: "+err.Error())
		return steps.Error(err)
	}
	// Resolve the energy metric template and apply it
	resolvedDataCenterResource, err = r.resolveEnergyMetricTemplate(ctx, resolvedDataCenterResource)
	if err != nil {
		dataCenterResource.UpdateStatusCondition(chantico.ConditionApplied, metav1.ConditionFalse, chantico.ReasonTemplateResolutionFailed, "Failed to resolve energy metric template: "+err.Error())
		return steps.Error(err)
	}
	ruleFile := dcr.BuildRuleFile(resolvedDataCenterResource)

	if ruleFile == nil {
		l.Info("No rule file found")
		if err := deleteRuleFile(dataCenterResource); err != nil {
			dataCenterResource.UpdateStatusCondition(chantico.ConditionApplied, metav1.ConditionFalse, chantico.ReasonCleanupFailed, "Error deleting rule file: "+err.Error())
			return steps.Error(err)
		}
		if err := reloadPrometheus(ctx); err != nil {
			dataCenterResource.UpdateStatusCondition(chantico.ConditionApplied, metav1.ConditionFalse, chantico.ReasonReloadFailed, "Failed to reload Prometheus: "+err.Error())
			return steps.Error(err)
		}
		dataCenterResource.UpdateStatusCondition(chantico.ConditionApplied, metav1.ConditionTrue, chantico.ReasonReconciled, "No recording rule file required")
		return steps.Continue()
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
	err = reloadPrometheus(ctx)
	if err != nil {
		dataCenterResource.UpdateStatusCondition(chantico.ConditionApplied, metav1.ConditionFalse, chantico.ReasonReloadFailed, "Failed to reload Prometheus: "+err.Error())
		return steps.Error(err)
	}

	dataCenterResource.UpdateStatusCondition(chantico.ConditionApplied, metav1.ConditionTrue, chantico.ReasonReconciled, "Recording rule file applied successfully")
	return steps.Continue()
}

// Helper function which resolves and performs the substitution for of the templates. Used for both the coefficient templates and energy metrics templates.
func (r *DataCenterResourceReconciler) resolveAndApplyTemplate(ctx context.Context, namespace string, templateFrom *chantico.TemplateFrom) (string, error) {
	// Lookup the configmap and resolve retrieve the template text
	configMap := &corev1.ConfigMap{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: templateFrom.ConfigMapKeyRef.Name}, configMap); err != nil {
		return "", fmt.Errorf("get template ConfigMap %q: %w", templateFrom.ConfigMapKeyRef.Name, err)
	}
	templateText, ok := configMap.Data[templateFrom.ConfigMapKeyRef.Key]
	if !ok {
		return "", fmt.Errorf("template ConfigMap %q does not contain key %q", templateFrom.ConfigMapKeyRef.Name, templateFrom.ConfigMapKeyRef.Key)
	}

	// Gather all of the template parameters and resolve their values
	parameters := make(map[string]string, len(templateFrom.Parameters))
	for _, parameter := range templateFrom.Parameters {
		value, err := r.resolveEnvVar(ctx, namespace, parameter)
		if err != nil {
			return "", fmt.Errorf("resolve template parameter %q: %w", parameter.Name, err)
		}
		parameters[parameter.Name] = value
	}

	// Apply the template with the resolved parameters
	tmpl, err := template.New(templateFrom.ConfigMapKeyRef.Name).Option("missingkey=error").Parse(templateText)
	if err != nil {
		return "", fmt.Errorf("parse template from ConfigMap %q: %w", templateFrom.ConfigMapKeyRef.Name, err)
	}
	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, parameters); err != nil {
		return "", fmt.Errorf("render template from ConfigMap %q: %w", templateFrom.ConfigMapKeyRef.Name, err)
	}
	return rendered.String(), nil
}

// Resolves the coefficient template for each parent in the DataCenterResource spec.
// The resolved template is put back into the `Coefficient` field of each parent.
func (r *DataCenterResourceReconciler) resolveCoefficientTemplates(ctx context.Context, dataCenterResource *chantico.DataCenterResource) (*chantico.DataCenterResource, error) {
	for index := range dataCenterResource.Spec.Parents {
		parent := &dataCenterResource.Spec.Parents[index]
		configMapRef := parent.CoefficientFrom.ConfigMapKeyRef
		if configMapRef.Name == "" && configMapRef.Key == "" {
			continue
		}
		if configMapRef.Name == "" || configMapRef.Key == "" {
			return nil, fmt.Errorf("parent %q coefficient template requires both configMapKeyRef.name and configMapKeyRef.key", parent.Name)
		}

		rendered, err := r.resolveAndApplyTemplate(ctx, dataCenterResource.Namespace, &parent.CoefficientFrom)
		if err != nil {
			return nil, fmt.Errorf("resolve and apply coefficient template for parent %q: %w", parent.Name, err)
		}

		// Write it back into the parent coefficient field
		parent.Coefficient = rendered
	}

	return dataCenterResource, nil
}

// Resolves the energy metric template and puts the value (after substituting in the template variables) into
// the `EnergyMetric` field of the DataCenterResource spec.
func (r *DataCenterResourceReconciler) resolveEnergyMetricTemplate(ctx context.Context, dataCenterResource *chantico.DataCenterResource) (*chantico.DataCenterResource, error) {
	if dataCenterResource.Spec.EnergyMetricFrom.ConfigMapKeyRef.Name == "" {
		return dataCenterResource, nil
	}
	rendered, err := r.resolveAndApplyTemplate(ctx, dataCenterResource.Namespace, &dataCenterResource.Spec.EnergyMetricFrom)
	if err != nil {
		return nil, fmt.Errorf("resolve and apply energy metric template: %w", err)
	}
	dataCenterResource.Spec.EnergyMetric = rendered

	return dataCenterResource, nil
}

// This uses the same struct as the kubernetes environment variable to resolve the parameters for a template.
// It resolves the value of the parameter in the envVar variable, either directly from the Value field,
// or from a ConfigMap or Secret if ValueFrom is specified.
func (r *DataCenterResourceReconciler) resolveEnvVar(ctx context.Context, namespace string, variable corev1.EnvVar) (string, error) {

	if variable.ValueFrom == nil {
		return variable.Value, nil
	}

	// Resolve the value from a config map
	if ref := variable.ValueFrom.ConfigMapKeyRef; ref != nil {
		configMap := &corev1.ConfigMap{}
		if err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: ref.Name}, configMap); err != nil {
			return "", fmt.Errorf("get ConfigMap %q: %w", ref.Name, err)
		}
		value, ok := configMap.Data[ref.Key]
		if !ok {
			return "", fmt.Errorf("ConfigMap %q does not contain key %q", ref.Name, ref.Key)
		}
		return value, nil
	}

	// Resolve the value from a secret
	if ref := variable.ValueFrom.SecretKeyRef; ref != nil {
		secret := &corev1.Secret{}
		if err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: ref.Name}, secret); err != nil {
			return "", fmt.Errorf("get Secret %q: %w", ref.Name, err)
		}
		value, ok := secret.Data[ref.Key]
		if !ok {
			return "", fmt.Errorf("Secret %q does not contain key %q", ref.Name, ref.Key)
		}
		return string(value), nil
	}
	return "", fmt.Errorf("must specify configMapKeyRef or secretKeyRef")
}

func (r *DataCenterResourceReconciler) reconcileReady(ctx context.Context, dataCenterResource *chantico.DataCenterResource) steps.StepResult {
	dataCenterResource.UpdateStatusCondition(chantico.ConditionReady, metav1.ConditionTrue, chantico.ReasonReconciled, "Fully reconciled and ready")
	return steps.Continue()
}

func deleteRuleFile(dataCenterResource *chantico.DataCenterResource) error {
	volumePath := config.ValidatedEnv.VolumeLocation
	rulePath := filepath.Join(volumePath, prometheusRulesDir, dataCenterResource.Name+".yml")
	if err := os.Remove(rulePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
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
		_ = patch.PatchStatus()
	}
}
func reloadPrometheus(ctx context.Context) error {
	l := log.FromContext(ctx)
	host := config.ValidatedEnv.PrometheusServiceHost
	port := config.ValidatedEnv.PrometheusServicePort

	sanitizedPrometheusURL := url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(host, port),
		Path:   "/-/reload",
	}
	resp, err := http.Post(sanitizedPrometheusURL.String(), "", nil)
	if err != nil {
		l.Error(err, "Failed to reload Prometheus")
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		l.Info("Prometheus reload returned status", "status", resp.StatusCode)
		return fmt.Errorf("prometheus reload returned status %d", resp.StatusCode)
	}
	l.Info("Prometheus configuration reloaded")
	return nil
}
