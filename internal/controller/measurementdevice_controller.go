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
	"fmt"
	"strconv"

	chantico "chantico/api/v1alpha1"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"

	config "chantico/internal/configuration"
	md "chantico/internal/measurementdevice"
	"chantico/internal/snmp"
	"chantico/internal/steps"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlcontroller "sigs.k8s.io/controller-runtime/pkg/controller"
	util "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	yaml "go.yaml.in/yaml/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	log "sigs.k8s.io/controller-runtime/pkg/log"
)

// +kubebuilder:rbac:groups=chantico-project.github.io,resources=measurementdevices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=chantico-project.github.io,resources=measurementdevices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=chantico-project.github.io,resources=measurementdevices/finalizers,verbs=create;update;patch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;patch;update;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="",resources=pods,verbs=list
// +kubebuilder:rbac:groups="",resources=pods/log,verbs=get

// MeasurementDeviceReconciler reconciles a MeasurementDevice
type MeasurementDeviceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Logs   md.JobLogReader
}

func (r *MeasurementDeviceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&chantico.MeasurementDevice{}).
		Owns(&batchv1.Job{}).
		Owns(&corev1.ConfigMap{}).
		WithOptions(ctrlcontroller.Options{MaxConcurrentReconciles: 1}). // Race conditions might occur when multiple generator jobs run simultaneously, so only allow one at a time.
		WithLogConstructor(func(req *reconcile.Request) logr.Logger {
			log := mgr.GetLogger().WithName("MeasurementDeviceController")
			if req != nil {
				log = log.WithValues("resource", req.Name)
			}
			return log
		}).
		Complete(r)
}

func (r *MeasurementDeviceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	l := log.FromContext(ctx)

	measurementDevice := &chantico.MeasurementDevice{}
	err := r.Get(ctx, req.NamespacedName, measurementDevice)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	l = l.WithValues("generation", measurementDevice.GetGeneration())
	ctx = log.IntoContext(ctx, l)

	// Patches the changes to the MeasurementDevice at the end of reconciliation. This updates the observedGeneration and conditions in the status.
	patcher, err := patch.NewHelper(measurementDevice, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}
	defer func() {
		if err := patcher.Patch(ctx, measurementDevice, patch.WithStatusObservedGeneration{}); err != nil {
			reterr = errors.Join(reterr, err)
		}
	}()

	measurementDevice.UpdateStatusCondition(chantico.ConditionReady, metav1.ConditionUnknown, chantico.ReasonReconciling, "Reconciliation is in progress")
	return steps.Run(ctx, measurementDevice,
		r.reconcileDeletion,
		r.ensureFinalizerIsSet,
		r.reconcileGeneratorConfigMap,
		r.reconcileSNMPGeneratorJob,
		r.reconcileSNMPFragment,
		r.reconcileMergedSNMPFile,
		r.reconcileExporterReload,
		r.reconcileReady,
	)
}

func (r *MeasurementDeviceReconciler) reconcileDeletion(ctx context.Context, measurementDevice *chantico.MeasurementDevice) steps.StepResult {
	if measurementDevice.GetDeletionTimestamp() == nil {
		return steps.Continue()
	}

	if !util.ContainsFinalizer(measurementDevice, chantico.SNMPUpdateFinalizer) {
		return steps.Stop()
	}

	log.FromContext(ctx).Info("Deleting MeasurementDevice SNMP configuration", "MeasurementDevice", measurementDevice.Name)
	jobs, err := r.getOwnedJobs(ctx, measurementDevice)
	if err != nil {
		measurementDevice.UpdateStatusCondition(chantico.ConditionApplied, metav1.ConditionFalse, chantico.ReasonCleanupFailed, "Failed to get owned jobs: "+err.Error())
		return steps.Error(err)
	}
	for i := range jobs {
		job := &jobs[i]
		if err := r.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground)); client.IgnoreNotFound(err) != nil {
			measurementDevice.UpdateStatusCondition(chantico.ConditionApplied, metav1.ConditionFalse, chantico.ReasonCleanupFailed, "Failed to delete owned job: "+err.Error())
			return steps.Error(err)
		}
	}

	fragment := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
		Name:      md.SnmpFragmentConfigMapName(measurementDevice.GetName()),
		Namespace: measurementDevice.GetNamespace(),
	}}
	if err := r.Delete(ctx, fragment); client.IgnoreNotFound(err) != nil {
		measurementDevice.UpdateStatusCondition(chantico.ConditionApplied, metav1.ConditionFalse, chantico.ReasonCleanupFailed, "Error while removing SNMP fragment ConfigMap: "+err.Error())
		return steps.Error(err)
	}

	if res := r.reconcileMergedSNMPFile(ctx, measurementDevice); res.Action == steps.ActionError {
		return res
	}
	if res := r.reconcileExporterReload(ctx, measurementDevice); res.Action == steps.ActionError {
		return res
	}

	util.RemoveFinalizer(measurementDevice, chantico.SNMPUpdateFinalizer)
	return steps.Stop()
}

func (r *MeasurementDeviceReconciler) ensureFinalizerIsSet(ctx context.Context, measurementDevice *chantico.MeasurementDevice) steps.StepResult {
	if util.ContainsFinalizer(measurementDevice, chantico.SNMPUpdateFinalizer) {
		return steps.Continue()
	}
	util.AddFinalizer(measurementDevice, chantico.SNMPUpdateFinalizer)
	return steps.Stop()
}

func (r *MeasurementDeviceReconciler) reconcileGeneratorConfigMap(ctx context.Context, measurementDevice *chantico.MeasurementDevice) steps.StepResult {
	desired, err := r.desiredGeneratorConfig(ctx, measurementDevice)
	if err != nil {
		measurementDevice.UpdateStatusCondition(chantico.ConditionGenerated, metav1.ConditionFalse, chantico.ReasonGenerationFailed, "Failed to build generator config: "+err.Error())
		return steps.Error(err)
	}

	configMap := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
		Name:      md.GeneratorConfigMapName(measurementDevice.GetName()),
		Namespace: measurementDevice.GetNamespace(),
	}}

	result, err := util.CreateOrUpdate(ctx, r.Client, configMap, func() error {
		configMap.Data = map[string]string{md.GeneratorConfigKey: string(desired)}
		return util.SetControllerReference(measurementDevice, configMap, r.Scheme)
	})
	if err != nil {
		measurementDevice.UpdateStatusCondition(chantico.ConditionGenerated, metav1.ConditionFalse, chantico.ReasonGenerationFailed, "Failed to apply generator ConfigMap "+configMap.Name+": "+err.Error())
		return steps.Error(err)
	}

	if result == util.OperationResultNone {
		measurementDevice.UpdateStatusCondition(chantico.ConditionGenerated, metav1.ConditionTrue, chantico.ReasonReconciled, "Generator ConfigMap is up to date")
		return steps.Continue()
	}

	log.FromContext(ctx).Info("Generator ConfigMap has been applied successfully.", "configMap", configMap.Name, "result", result)
	measurementDevice.UpdateStatusCondition(chantico.ConditionGenerated, metav1.ConditionTrue, chantico.ReasonReconciled, "Generator ConfigMap has been applied successfully")
	return steps.Continue()
}

func (r *MeasurementDeviceReconciler) desiredGeneratorConfig(ctx context.Context, measurementDevice *chantico.MeasurementDevice) ([]byte, error) {
	authString, err := measurementDevice.Spec.AuthFrom.Resolve(ctx, r.Client, measurementDevice.Namespace)

	if err != nil {
		return []byte{}, err
	}

	var ga snmp.GeneratorAuth

	err = yaml.Unmarshal([]byte(authString), &ga)

	if err != nil {
		return []byte{}, err
	}
	return yaml.Marshal(
		snmp.GeneratorConfig{
			Auths:   map[string]*snmp.GeneratorAuth{measurementDevice.Name: &ga},
			Modules: map[string]*snmp.GeneratorModule{measurementDevice.Name: {Walk: measurementDevice.Spec.Walks}},
		},
	)
}

func (r *MeasurementDeviceReconciler) reconcileSNMPGeneratorJob(ctx context.Context, measurementDevice *chantico.MeasurementDevice) steps.StepResult {
	jobs, err := r.getOwnedJobs(ctx, measurementDevice)
	if err != nil {
		measurementDevice.UpdateStatusCondition(chantico.ConditionGenerated, metav1.ConditionFalse, chantico.ReasonGenerationFailed, "Failed to get owned SNMP Generator jobs: "+err.Error())
		return steps.Error(err)
	}

	switch len(jobs) {
	case 0:
		return r.createGeneratorJob(ctx, measurementDevice)
	case 1:
		return r.evaluateGeneratorJob(ctx, measurementDevice, &jobs[0])
	default:
		measurementDevice.UpdateStatusCondition(chantico.ConditionGenerated, metav1.ConditionFalse, chantico.ReasonGenerationFailed, "Expected at most 1 owned job, found "+strconv.Itoa(len(jobs)))
		return steps.Error(fmt.Errorf("expected at most 1 owned job, found %d", len(jobs)))
	}
}

func (r *MeasurementDeviceReconciler) createGeneratorJob(
	ctx context.Context, measurementDevice *chantico.MeasurementDevice,
) steps.StepResult {
	job, err := md.BuildGeneratorJob(measurementDevice)
	if err != nil {
		measurementDevice.UpdateStatusCondition(chantico.ConditionGenerated, metav1.ConditionFalse, chantico.ReasonGenerationFailed, "Failed to build SNMP Generator job: "+err.Error())
		return steps.Error(err)
	}
	if err := util.SetControllerReference(measurementDevice, job, r.Scheme); err != nil {
		measurementDevice.UpdateStatusCondition(chantico.ConditionGenerated, metav1.ConditionFalse, chantico.ReasonGenerationFailed, "Failed to set controller reference for SNMP Generator job: "+err.Error())
		return steps.Error(err)
	}
	if err := r.Create(ctx, job); err != nil {
		measurementDevice.UpdateStatusCondition(chantico.ConditionGenerated, metav1.ConditionFalse, chantico.ReasonGenerationFailed, "Failed to create SNMP Generator job: "+err.Error())
		return steps.Error(err)
	}

	log.FromContext(ctx).Info("Created SNMP Generator job", "job", job.Name)
	measurementDevice.UpdateStatusCondition(chantico.ConditionGenerated, metav1.ConditionUnknown, chantico.ReasonGenerationPending, "SNMP Generator Job created")
	return steps.Stop()
}

func (r *MeasurementDeviceReconciler) evaluateGeneratorJob(ctx context.Context, measurementDevice *chantico.MeasurementDevice, job *batchv1.Job) steps.StepResult {
	l := log.FromContext(ctx)

	if jobGeneration(job) != measurementDevice.GetGeneration() {
		l.Info("Stale SNMP Generator job, deleting...", "job", job.Name)
		if err := r.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground)); client.IgnoreNotFound(err) != nil {
			measurementDevice.UpdateStatusCondition(chantico.ConditionGenerated, metav1.ConditionFalse, chantico.ReasonGenerationFailed, "Failed to delete stale SNMP Generator job "+job.Name+": "+err.Error())
			return steps.Error(err)
		}
		return steps.Stop()
	}

	switch {
	case isJobSuccessful(job):
		l.Info("Generator job succeeded", "job", job.Name)
		measurementDevice.UpdateStatusCondition(chantico.ConditionGenerated, metav1.ConditionUnknown, chantico.ReasonGenerationPending, "SNMP Generator Job succeeded; generated config is being verified")
		return steps.Continue()
	case isJobFailed(job):
		l.Info("Generator job failed", "job", job.Name)
		measurementDevice.UpdateStatusCondition(chantico.ConditionGenerated, metav1.ConditionFalse, chantico.ReasonGenerationFailed, "SNMP Generator Job failed")
		return steps.Stop()
	default:
		l.Info("Generator job is running", "job", job.Name)
		measurementDevice.UpdateStatusCondition(chantico.ConditionGenerated, metav1.ConditionUnknown, chantico.ReasonGenerationPending, "SNMP Generator Job is running")
		return steps.Stop()
	}
}

// reconcileSNMPFragment stores the config generated by the Job in the fragment ConfigMap of the device.
// The log of the Job is only read when the fragment does not hold the config of this generation yet.
func (r *MeasurementDeviceReconciler) reconcileSNMPFragment(ctx context.Context, measurementDevice *chantico.MeasurementDevice) steps.StepResult {
	generation := strconv.FormatInt(measurementDevice.GetGeneration(), 10)
	fragment := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
		Name:      md.SnmpFragmentConfigMapName(measurementDevice.GetName()),
		Namespace: measurementDevice.GetNamespace(),
	}}

	current := &corev1.ConfigMap{}
	getErr := r.Get(ctx, client.ObjectKeyFromObject(fragment), current)
	if client.IgnoreNotFound(getErr) != nil {
		measurementDevice.UpdateStatusCondition(chantico.ConditionGenerated, metav1.ConditionFalse, chantico.ReasonGenerationFailed, "Failed to read SNMP fragment ConfigMap "+fragment.Name+": "+getErr.Error())
		return steps.Error(getErr)
	}

	content := current.Data[md.SnmpConfigKey]
	if getErr != nil || current.Annotations[md.GenerationAnnotation] != generation {
		output, err := r.generatedSNMPConfig(ctx, measurementDevice)
		if err != nil {
			measurementDevice.UpdateStatusCondition(chantico.ConditionGenerated, metav1.ConditionFalse, chantico.ReasonGenerationFailed, "Failed to read generated SNMP config: "+err.Error())
			return steps.Error(err)
		}

		if _, err := util.CreateOrUpdate(ctx, r.Client, fragment, func() error {
			fragment.Labels = map[string]string{md.SnmpFragmentLabel: "true"}
			fragment.Annotations = map[string]string{md.GenerationAnnotation: generation}
			fragment.Data = map[string]string{md.SnmpConfigKey: string(output)}
			return util.SetControllerReference(measurementDevice, fragment, r.Scheme)
		}); err != nil {
			measurementDevice.UpdateStatusCondition(chantico.ConditionGenerated, metav1.ConditionFalse, chantico.ReasonGenerationFailed, "Failed to apply SNMP fragment ConfigMap "+fragment.Name+": "+err.Error())
			return steps.Error(err)
		}
		content = string(output)
	}

	configHash := snmp.Hash([]byte(content))

	if measurementDevice.Status.ConfigHash == configHash {
		measurementDevice.UpdateStatusCondition(chantico.ConditionGenerated, metav1.ConditionTrue, chantico.ReasonReconciled, "ConfigHash matches with SNMP configuration")
		return steps.Continue()
	}

	measurementDevice.Status.ConfigHash = configHash
	measurementDevice.UpdateStatusCondition(chantico.ConditionGenerated, metav1.ConditionTrue, chantico.ReasonReconciled, "ConfigHash has been updated to match with SNMP configuration")
	return steps.Continue()
}

// generatedSNMPConfig returns the SNMP config that the generator Job of the device printed to its log.
func (r *MeasurementDeviceReconciler) generatedSNMPConfig(ctx context.Context, measurementDevice *chantico.MeasurementDevice) ([]byte, error) {
	jobs, err := r.getOwnedJobs(ctx, measurementDevice)
	if err != nil {
		return nil, err
	}
	if len(jobs) != 1 {
		return nil, fmt.Errorf("expected 1 owned job, found %d", len(jobs))
	}

	output, err := r.Logs.GetJobLogs(ctx, &jobs[0])
	if err != nil {
		return nil, err
	}

	// A broken config would break the merged config of all devices.
	if len(bytes.TrimSpace(output)) == 0 {
		return nil, errors.New("the generator job log is empty")
	}
	if err := yaml.Unmarshal(output, &snmp.MergedConfig{}); err != nil {
		return nil, fmt.Errorf("the generator job log is not a valid SNMP config: %w", err)
	}
	return output, nil
}

// mergedSNMPConfig merges the SNMP fragments of all devices. When the given device is being
// deleted its fragment is left out, whether or not it has been removed from the cache yet.
func (r *MeasurementDeviceReconciler) mergedSNMPConfig(ctx context.Context, measurementDevice *chantico.MeasurementDevice) ([]byte, error) {
	fragments := &corev1.ConfigMapList{}
	if err := r.List(ctx, fragments, client.MatchingLabels{md.SnmpFragmentLabel: "true"}); err != nil {
		return nil, err
	}

	configs := map[string][]byte{}
	for i := range fragments.Items {
		fragment := &fragments.Items[i]
		if fragment.GetDeletionTimestamp() != nil {
			continue
		}
		if measurementDevice.GetDeletionTimestamp() != nil && metav1.IsControlledBy(fragment, measurementDevice) {
			continue
		}
		configs[fragment.Namespace+"/"+fragment.Name] = []byte(fragment.Data[md.SnmpConfigKey])
	}
	return snmp.MergeSortedSNMPConfigs(configs)
}

func (r *MeasurementDeviceReconciler) reconcileMergedSNMPFile(ctx context.Context, measurementDevice *chantico.MeasurementDevice) steps.StepResult {
	merged, err := r.mergedSNMPConfig(ctx, measurementDevice)
	if err != nil {
		measurementDevice.UpdateStatusCondition(chantico.ConditionApplied, metav1.ConditionFalse, chantico.ReasonApplyFailed, "Failed to read SNMP configs: "+err.Error())
		return steps.Error(err)
	}

	configMap := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
		Name:      md.MergedConfigMapName,
		Namespace: config.ValidatedEnv.PodNamespace,
	}}
	result, err := util.CreateOrUpdate(ctx, r.Client, configMap, func() error {
		configMap.Data = map[string]string{md.SnmpConfigKey: string(merged)}
		return nil
	})
	if err != nil {
		measurementDevice.UpdateStatusCondition(chantico.ConditionApplied, metav1.ConditionFalse, chantico.ReasonApplyFailed, "Failed to apply merged SNMP ConfigMap "+configMap.Name+": "+err.Error())
		return steps.Error(err)
	}

	if result == util.OperationResultNone {
		measurementDevice.UpdateStatusCondition(chantico.ConditionApplied, metav1.ConditionUnknown, chantico.ReasonReconciling, "Merged SNMP ConfigMap is up to date; exporter reload is being verified")
		return steps.Continue()
	}

	measurementDevice.UpdateStatusCondition(chantico.ConditionApplied, metav1.ConditionUnknown, chantico.ReasonReconciling, "Merged SNMP ConfigMap has been applied successfully; exporter reload is pending")
	return steps.Continue()
}

func (r *MeasurementDeviceReconciler) reconcileExporterReload(ctx context.Context, measurementDevice *chantico.MeasurementDevice) steps.StepResult {
	merged, err := r.mergedSNMPConfig(ctx, measurementDevice)
	if err != nil {
		measurementDevice.UpdateStatusCondition(chantico.ConditionApplied, metav1.ConditionFalse, chantico.ReasonApplyFailed, "Failed to read SNMP configs: "+err.Error())
		return steps.Error(err)
	}

	desiredHash := snmp.Hash(merged)

	exporter, err := r.getSnmpExporterDeployment(ctx)
	if err != nil {
		measurementDevice.UpdateStatusCondition(chantico.ConditionApplied, metav1.ConditionFalse, chantico.ReasonDependencyUnavailable, "Failed to get SNMP exporter deployment: "+err.Error())
		return steps.Error(err)
	}

	current := exporter.Spec.Template.Annotations[md.ConfigHashAnnotation]
	if current == desiredHash {
		measurementDevice.UpdateStatusCondition(chantico.ConditionApplied, metav1.ConditionTrue, chantico.ReasonReconciled, "SNMP exporter is up to date with merged config.")
		return steps.Continue()
	}

	patch := client.MergeFrom(exporter.DeepCopy())
	if exporter.Spec.Template.Annotations == nil {
		exporter.Spec.Template.Annotations = map[string]string{}
	}
	exporter.Spec.Template.Annotations[md.ConfigHashAnnotation] = desiredHash
	if err := r.Patch(ctx, exporter, patch); err != nil {
		measurementDevice.UpdateStatusCondition(chantico.ConditionApplied, metav1.ConditionFalse, chantico.ReasonApplyFailed, "Failed to patch SNMP exporter deployment "+exporter.Name+": "+err.Error())
		return steps.Error(err)
	}

	log.FromContext(ctx).Info("Triggered SNMP exporter reload", "hash", desiredHash)
	measurementDevice.UpdateStatusCondition(chantico.ConditionApplied, metav1.ConditionTrue, chantico.ReasonReconciled, "SNMP exporter deployment annotation updated to trigger reload")
	return steps.Continue()
}

func (r *MeasurementDeviceReconciler) reconcileReady(ctx context.Context, measurementDevice *chantico.MeasurementDevice) steps.StepResult {
	measurementDevice.UpdateStatusCondition(chantico.ConditionReady, metav1.ConditionTrue, chantico.ReasonReconciled, "Fully reconciled and ready")
	return steps.Continue()
}

func (r *MeasurementDeviceReconciler) getSnmpExporterDeployment(ctx context.Context) (*appsv1.Deployment, error) {
	var deploy appsv1.Deployment
	if err := r.Get(ctx, client.ObjectKey{Name: "chantico-snmp", Namespace: config.ValidatedEnv.PodNamespace}, &deploy); err != nil {
		return nil, err
	}
	return &deploy, nil
}

func jobGeneration(job *batchv1.Job) int64 {
	s := job.GetAnnotations()[md.GenerationAnnotation]
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

func isJobFailed(job *batchv1.Job) bool {
	for _, condition := range job.Status.Conditions {
		if condition.Type == batchv1.JobFailed && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func isJobSuccessful(job *batchv1.Job) bool {
	for _, condition := range job.Status.Conditions {
		if condition.Type == batchv1.JobComplete && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func (r *MeasurementDeviceReconciler) getOwnedJobs(ctx context.Context, measurementDevice *chantico.MeasurementDevice) ([]batchv1.Job, error) {
	jobList := &batchv1.JobList{}
	if err := r.List(ctx, jobList, client.InNamespace(measurementDevice.GetNamespace())); err != nil {
		return nil, err
	}

	var ownedJobs []batchv1.Job
	for _, job := range jobList.Items {
		for _, ownerRef := range job.OwnerReferences {
			if ownerRef.UID == measurementDevice.GetUID() {
				ownedJobs = append(ownedJobs, job)
			}
		}
	}
	return ownedJobs, nil
}
