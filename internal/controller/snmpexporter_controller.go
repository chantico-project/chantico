package controller

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	chantico "chantico/api/v1alpha1"
	"chantico/internal/config"
	"chantico/internal/snmpexporter"
	"chantico/internal/snmpgenerator"
	"chantico/internal/steps"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// +kubebuilder:rbac:groups=chantico.ci.tno.nl,resources=snmpexporters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=chantico.ci.tno.nl,resources=snmpexporters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=chantico.ci.tno.nl,resources=snmpexporters/finalizers,verbs=create;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete

type SNMPExporterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Config config.Config
	Paths  snmpgenerator.Paths // shared with the generator controller
}

func (r *SNMPExporterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&chantico.SNMPExporter{}).
		Owns(&appsv1.Deployment{}).
		Watches(
			&chantico.SNMPDevice{},
			handler.EnqueueRequestsFromMapFunc(r.mapDeviceToExporters),
			builder.WithPredicates(),
		).
		Complete(r)
}

// mapDeviceToExporters re-enqueues every SNMPExporter in the device's
// namespace whenever a device changes. Filtering by selector happens
// inside the reconcile.
func (r *SNMPExporterReconciler) mapDeviceToExporters(ctx context.Context, obj client.Object) []reconcile.Request {
	var list chantico.SNMPExporterList
	if err := r.List(ctx, &list, client.InNamespace(obj.GetNamespace())); err != nil {
		return nil
	}
	out := make([]reconcile.Request, 0, len(list.Items))
	for _, e := range list.Items {
		out = append(out, reconcile.Request{NamespacedName: types.NamespacedName{
			Name: e.Name, Namespace: e.Namespace,
		}})
	}
	return out
}

func (r *SNMPExporterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	exporter := &chantico.SNMPExporter{}
	if err := r.Get(ctx, req.NamespacedName, exporter); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	helper, err := patch.NewHelper(exporter, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}
	defer func() {
		if err := r.reconcileReady(exporter); err != nil {
			reterr = errors.Join(reterr, err)
		}
		if err := helper.Patch(ctx, exporter); err != nil {
			reterr = errors.Join(reterr, err)
		}
	}()

	return steps.Run(ctx, exporter,
		r.reconcileDeletion,
		r.ensureFinalizer,
		r.reconcileMergedSNMPFile,
		r.reconcileExporterDeployment,
		r.setObservedGeneration,
	)
}

func (r *SNMPExporterReconciler) reconcileDeletion(ctx context.Context, e *chantico.SNMPExporter) steps.StepResult {
	if e.GetDeletionTimestamp() == nil {
		return steps.Continue()
	}
	if !controllerutil.ContainsFinalizer(e, chantico.SNMPExporterFinalizer) {
		return steps.Stop()
	}
	// Owned Deployment is garbage-collected by the API server.
	// Remove the merged snmp.yml so a future exporter starts clean.
	mergedPath := filepath.Join(r.Paths.SNMPDir(), "snmp.yml")
	if err := os.Remove(mergedPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return steps.Error(fmt.Errorf("remove %s: %w", mergedPath, err))
	}
	controllerutil.RemoveFinalizer(e, chantico.SNMPExporterFinalizer)
	return steps.Stop()
}

func (r *SNMPExporterReconciler) ensureFinalizer(ctx context.Context, e *chantico.SNMPExporter) steps.StepResult {
	if controllerutil.ContainsFinalizer(e, chantico.SNMPExporterFinalizer) {
		return steps.Continue()
	}
	controllerutil.AddFinalizer(e, chantico.SNMPExporterFinalizer)
	return steps.Continue()
}

func (r *SNMPExporterReconciler) reconcileMergedSNMPFile(ctx context.Context, e *chantico.SNMPExporter) steps.StepResult {
	devices, err := r.selectDevices(ctx, e)
	if err != nil {
		return steps.Error(err)
	}

	fragments := map[string][]byte{}
	for i := range devices {
		d := &devices[i]
		if !deviceConfigReady(d) {
			continue // skip devices whose per-device file isn't trustworthy yet
		}
		path := r.Paths.SNMPFile(d.GetUID())
		content, err := os.ReadFile(path)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return steps.Error(fmt.Errorf("read %s: %w", path, err))
		}
		fragments[d.GetName()] = content
	}

	merged, err := snmpexporter.Merge(snmpexporter.SortedFragments(fragments))
	if err != nil {
		e.UpdateStatusCondition(chantico.ConditionMergedConfig, metav1.ConditionFalse,
			chantico.ReasonMergeFailed, err.Error())
		return steps.Error(err)
	}

	e.Status.DeviceCount = int32(len(fragments))
	e.Status.ConfigHash = snmpexporter.Hash(merged)

	mergedPath := filepath.Join(r.Paths.SNMPDir(), "snmp.yml")
	if err := os.MkdirAll(filepath.Dir(mergedPath), 0o755); err != nil {
		return steps.Error(err)
	}
	if err := os.WriteFile(mergedPath, merged, 0o644); err != nil {
		e.UpdateStatusCondition(chantico.ConditionMergedConfig, metav1.ConditionFalse,
			chantico.ReasonMergeFailed, err.Error())
		return steps.Error(fmt.Errorf("write %s: %w", mergedPath, err))
	}

	e.UpdateStatusCondition(chantico.ConditionMergedConfig, metav1.ConditionTrue,
		chantico.ReasonMerged, fmt.Sprintf("merged %d device(s)", len(fragments)))
	return steps.Continue()
}

func (r *SNMPExporterReconciler) reconcileExporterDeployment(ctx context.Context, e *chantico.SNMPExporter) steps.StepResult {
	desired, err := snmpexporter.BuildDeployment(r.Config, e, e.Status.ConfigHash)
	if err != nil {
		return steps.Error(err)
	}
	if err := controllerutil.SetControllerReference(e, desired, r.Scheme); err != nil {
		return steps.Error(err)
	}

	existing := &appsv1.Deployment{}
	err = r.Get(ctx, client.ObjectKeyFromObject(desired), existing)
	switch {
	case apierrors.IsNotFound(err):
		if err := r.Create(ctx, desired); err != nil {
			return steps.Error(fmt.Errorf("create deployment: %w", err))
		}
		e.UpdateStatusCondition(chantico.ConditionDeployment, metav1.ConditionUnknown,
			chantico.ReasonRollingOut, "Deployment created")
		return steps.Stop()
	case err != nil:
		return steps.Error(err)
	}

	// Update only the fields we own. Use a server-side apply or a
	// simple spec replace; below is the simple form.
	existing.Spec = desired.Spec
	existing.Labels = desired.Labels
	if err := r.Update(ctx, existing); err != nil {
		return steps.Error(fmt.Errorf("update deployment: %w", err))
	}

	if isDeploymentAvailable(existing) {
		e.UpdateStatusCondition(chantico.ConditionDeployment, metav1.ConditionTrue,
			chantico.ReasonDeploymentReady, "Deployment is available")
		return steps.Continue()
	}
	e.UpdateStatusCondition(chantico.ConditionDeployment, metav1.ConditionUnknown,
		chantico.ReasonRollingOut, "Deployment rolling out")
	return steps.Stop()
}

func (r *SNMPExporterReconciler) setObservedGeneration(ctx context.Context, e *chantico.SNMPExporter) steps.StepResult {
	e.Status.ObservedGeneration = e.GetGeneration()
	return steps.Continue()
}

func (r *SNMPExporterReconciler) reconcileReady(e *chantico.SNMPExporter) error {
	merged := metav1ConditionStatus(e, chantico.ConditionMergedConfig)
	deploy := metav1ConditionStatus(e, chantico.ConditionDeployment)
	switch {
	case merged == metav1.ConditionTrue && deploy == metav1.ConditionTrue:
		e.UpdateStatusCondition(chantico.ConditionReady, metav1.ConditionTrue,
			chantico.ReasonSucceeded, "SNMPExporter is ready")
	case merged == metav1.ConditionFalse || deploy == metav1.ConditionFalse:
		e.UpdateStatusCondition(chantico.ConditionReady, metav1.ConditionFalse,
			chantico.ReasonFailed, "SNMPExporter has failing components")
	default:
		e.UpdateStatusCondition(chantico.ConditionReady, metav1.ConditionUnknown,
			chantico.ReasonPending, "SNMPExporter is not ready yet")
	}
	return nil
}

func (r *SNMPExporterReconciler) selectDevices(ctx context.Context, e *chantico.SNMPExporter) ([]chantico.SNMPDevice, error) {
	selector := labels.Everything()
	if e.Spec.DeviceSelector != nil {
		s, err := metav1.LabelSelectorAsSelector(e.Spec.DeviceSelector)
		if err != nil {
			return nil, err
		}
		selector = s
	}
	var list chantico.SNMPDeviceList
	if err := r.List(ctx, &list,
		client.InNamespace(e.GetNamespace()),
		client.MatchingLabelsSelector{Selector: selector},
	); err != nil {
		return nil, err
	}
	return list.Items, nil
}

// deviceConfigReady reports whether the device's per-device snmp file
// reflects the current spec generation.
func deviceConfigReady(d *chantico.SNMPDevice) bool {
	if d.Status.ObservedGeneration != d.GetGeneration() {
		return false
	}
	cond := meta.FindStatusCondition(d.Status.Conditions, string(chantico.ConditionConfig))
	return cond != nil && cond.Status == metav1.ConditionTrue
}

func isDeploymentAvailable(d *appsv1.Deployment) bool {
	for _, c := range d.Status.Conditions {
		if c.Type == appsv1.DeploymentAvailable && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func metav1ConditionStatus(e *chantico.SNMPExporter, t chantico.ConditionType) metav1.ConditionStatus {
	c := meta.FindStatusCondition(e.Status.Conditions, string(t))
	if c == nil {
		return metav1.ConditionUnknown
	}
	return c.Status
}
