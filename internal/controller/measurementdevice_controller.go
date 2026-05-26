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
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"

	chantico "chantico/api/v1alpha1"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"

	md "chantico/internal/measurementdevice"
	"chantico/internal/snmp"
	"chantico/internal/steps"
	"crypto/sha256"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlcontroller "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;update;patch

// MeasurementDeviceReconciler reconciles a MeasurementDevice
type MeasurementDeviceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Paths  md.Paths
}

func (r *MeasurementDeviceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&chantico.MeasurementDevice{}).
		Owns(&batchv1.Job{}).
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

	// Helper function makes a deep copy of MeasurementDevice, and Patches the spec/status as needed at the end of reconcile function.
	helper, err := patch.NewHelper(measurementDevice, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}
	defer func() {
		if err := helper.Patch(ctx, measurementDevice); err != nil {
			reterr = errors.Join(reterr, err)
		}
	}()

	return steps.Run(ctx, measurementDevice,
		r.reconcileDeletion,
		r.ensureFinalizerIsSet,
		r.reconcileGeneratorFile,
		r.reconcileSNMPGeneratorJob,
		r.reconcileSNMPFileContent,
		r.reconcileMergedSNMPFile,
		r.reconcileExporterReload,
		r.setObservedGeneration,
	)
}

func (r *MeasurementDeviceReconciler) reconcileDeletion(ctx context.Context, measurementDevice *chantico.MeasurementDevice) steps.StepResult {
	l := log.FromContext(ctx)
	if measurementDevice.ObjectMeta.GetDeletionTimestamp() == nil {
		return steps.Continue()
	}

	if !util.ContainsFinalizer(measurementDevice, chantico.SNMPUpdateFinalizer) {
		return steps.Stop()
	}

	l.Info("Deleting MeasurementDevice files", "MeasurementDevice", measurementDevice.Name)
	jobs, err := r.getOwnedJobs(ctx, measurementDevice)
	if err != nil {
		return steps.Error(err)
	}
	for i := range jobs {
		job := &jobs[i]
		if err := r.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground)); client.IgnoreNotFound(err) != nil {
			return steps.Error(fmt.Errorf("delete owned job %s: %w", job.Name, err))
		}
	}

	filesToRemove := []string{
		r.Paths.GeneratorFile(measurementDevice.GetUID()),
		r.Paths.SNMPFile(measurementDevice.GetUID()),
	}
	for _, path := range filesToRemove {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return steps.Error(fmt.Errorf("Error while removing SNMP file %s: %w", path, err))
		}
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

func (r *MeasurementDeviceReconciler) reconcileGeneratorFile(ctx context.Context, measurementDevice *chantico.MeasurementDevice) steps.StepResult {
	l := log.FromContext(ctx)

	path := r.Paths.GeneratorFile(measurementDevice.GetUID())

	observed, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		l.Error(err, "Cannot read generator file", "path", path)
		return steps.Error(fmt.Errorf("read generator file %s: %w", path, err))
	}

	desired, err := desiredGeneratorConfig(measurementDevice)
	if err != nil {
		measurementDevice.UpdateStatusCondition(chantico.ConditionGeneratorFile, metav1.ConditionFalse, chantico.ReasonFailed, fmt.Sprintf("failed to marshal generator config: %v", err))
		return steps.Error(err)
	}

	if bytes.Equal(observed, desired) {
		measurementDevice.UpdateStatusCondition(chantico.ConditionGeneratorFile, metav1.ConditionTrue, chantico.ReasonSynced, "Generator file is up to date.")
		return steps.Continue()
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0777); err != nil {
		return steps.Error(err)
	}

	if err := os.WriteFile(path, desired, 0777); err != nil {
		l.Error(err, "Cannot write generator file", "path", path)
		measurementDevice.UpdateStatusCondition(chantico.ConditionGeneratorFile, metav1.ConditionFalse, chantico.ReasonFailed, fmt.Sprintf("failed to write generator file: %v", err))
		return steps.Error(fmt.Errorf("write generator file %s: %w", path, err))
	}

	l.Info("Generator file has been generated successfully.", "path", path)
	measurementDevice.UpdateStatusCondition(chantico.ConditionGeneratorFile, metav1.ConditionTrue, chantico.ReasonFileWritten, "Generator file has been generated successfully.")
	return steps.Continue()
}

func desiredGeneratorConfig(measurementDevice *chantico.MeasurementDevice) ([]byte, error) {
	return yaml.Marshal(snmp.GeneratorConfig{
		Auths:   map[string]*snmp.GeneratorAuth{measurementDevice.Name: &measurementDevice.Spec.Auth},
		Modules: map[string]*snmp.GeneratorModule{measurementDevice.Name: {Walk: measurementDevice.Spec.Walks}},
	})
}

func (r *MeasurementDeviceReconciler) reconcileSNMPGeneratorJob(ctx context.Context, measurementDevice *chantico.MeasurementDevice) steps.StepResult {
	if err := os.MkdirAll(r.Paths.SNMPDir(), 0777); err != nil {
		return steps.Error(fmt.Errorf("Error while creating SNMP folder: %w", err))
	}

	jobs, err := r.getOwnedJobs(ctx, measurementDevice)
	if err != nil {
		return steps.Error(err)
	}

	switch len(jobs) {
	case 0:
		return r.createGeneratorJob(ctx, measurementDevice)
	case 1:
		return r.evaluateGeneratorJob(ctx, measurementDevice, &jobs[0])
	default:
		return steps.Error(fmt.Errorf("measurementdevice %s owns %d jobs, expected at most 1", measurementDevice.GetName(), len(jobs)))
	}
}

func (r *MeasurementDeviceReconciler) createGeneratorJob(
	ctx context.Context, measurementDevice *chantico.MeasurementDevice,
) steps.StepResult {
	l := log.FromContext(ctx)
	job, err := md.BuildGeneratorJob(measurementDevice)
	if err != nil {
		return steps.Error(err)
	}
	if err := controllerutil.SetControllerReference(measurementDevice, job, r.Scheme); err != nil {
		return steps.Error(err)
	}
	if err := r.Create(ctx, job); err != nil {
		return steps.Error(fmt.Errorf("create generator job: %w", err))
	}

	l.Info("Created SNMP Generator job", "job", job.Name)
	measurementDevice.UpdateStatusCondition(chantico.ConditionJob, metav1.ConditionUnknown, chantico.ReasonPending, "SNMP Generator Job created")
	return steps.Stop()
}

func (r *MeasurementDeviceReconciler) evaluateGeneratorJob(ctx context.Context, measurementDevice *chantico.MeasurementDevice, job *batchv1.Job) steps.StepResult {
	l := log.FromContext(ctx)

	if jobGeneration(job) != measurementDevice.GetGeneration() {
		// stale — delete and let the next reconcile recreate.\
		l.Info("Stale SNMP Generator job, deleting...", "job", job.Name)
		if err := r.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground)); client.IgnoreNotFound(err) != nil {
			return steps.Error(fmt.Errorf("delete stale job: %w", err))
		}
		return steps.Stop()
	}

	switch {
	case isJobSuccessful(job):
		l.Info("Generator job succeeded", "job", job.Name)
		measurementDevice.UpdateStatusCondition(chantico.ConditionJob, metav1.ConditionTrue, chantico.ReasonSucceeded, "SNMP Generator Job succeeded")
		return steps.Continue()
	case isJobFailed(job):
		l.Info("Generator job failed", "job", job.Name)
		measurementDevice.UpdateStatusCondition(chantico.ConditionJob, metav1.ConditionFalse, chantico.ReasonFailed, "SNMP Generator Job failed")
		return steps.Stop()
	default:
		measurementDevice.UpdateStatusCondition(chantico.ConditionJob, metav1.ConditionUnknown, chantico.ReasonPending, "SNMP Generator Job is running")
		return steps.Stop()
	}
}

func (r *MeasurementDeviceReconciler) reconcileSNMPFileContent(ctx context.Context, measurementDevice *chantico.MeasurementDevice) steps.StepResult {
	path := r.Paths.SNMPFile(measurementDevice.GetUID())
	config, err := os.ReadFile(path)
	if err != nil {
		return steps.Error(err)
	}

	configSha := sha256.Sum256(config)
	configHash := hex.EncodeToString(configSha[:])

	if measurementDevice.Status.ConfigHash == configHash {
		measurementDevice.UpdateStatusCondition(chantico.ConditionConfig, metav1.ConditionTrue, chantico.ReasonSucceeded, "ConfigHash matches with SNMP configuration")
		return steps.Continue()
	}

	measurementDevice.Status.ConfigHash = configHash
	measurementDevice.UpdateStatusCondition(chantico.ConditionConfig, metav1.ConditionTrue, chantico.ReasonSynced, "ConfigHash has been updated to match with SNMP configuration")
	return steps.Continue()
}

func (r *MeasurementDeviceReconciler) reconcileMergedSNMPFile(ctx context.Context, measurementDevice *chantico.MeasurementDevice) steps.StepResult {
	fail := func(err error, msg string) steps.StepResult {
		log.FromContext(ctx).Error(err, msg)
		measurementDevice.UpdateStatusCondition(chantico.ConditionConfig, metav1.ConditionFalse, chantico.ReasonFailed, fmt.Sprintf("%s: %v", msg, err))
		return steps.Error(fmt.Errorf("%s: %w", msg, err))
	}

	merged, err := snmp.GetMergedSortedSNMPConfig(r.Paths.SNMPDir())
	if err != nil {
		return fail(err, "read per-device SNMP configs")
	}

	path := r.Paths.MergedSNMPFile()
	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fail(err, fmt.Sprintf("read %s", path))
	}
	if bytes.Equal(existing, merged) {
		measurementDevice.UpdateStatusCondition(chantico.ConditionConfig, metav1.ConditionTrue, chantico.ReasonSynced, "Merged SNMP file is up to date.")
		return steps.Continue()
	}

	if err := os.MkdirAll(filepath.Dir(path), 0777); err != nil {
		return fail(err, "create merged SNMP dir")
	}

	// Atomic write.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, merged, 0777); err != nil {
		return fail(err, fmt.Sprintf("write %s", tmp))
	}
	if err := os.Rename(tmp, path); err != nil {
		return fail(err, fmt.Sprintf("rename %s -> %s", tmp, path))
	}

	measurementDevice.UpdateStatusCondition(chantico.ConditionConfig, metav1.ConditionTrue, chantico.ReasonFileWritten, "Merged SNMP file has been written successfully.")
	return steps.Continue()
}

func (r *MeasurementDeviceReconciler) reconcileExporterReload(ctx context.Context, measurementDevice *chantico.MeasurementDevice) steps.StepResult {
	l := log.FromContext(ctx)
	fail := func(err error, msg string) steps.StepResult {
		log.FromContext(ctx).Error(err, msg)
		measurementDevice.UpdateStatusCondition(chantico.ConditionExporterReload, metav1.ConditionFalse, chantico.ReasonFailed, fmt.Sprintf("%s: %v", msg, err))
		return steps.Error(fmt.Errorf("%s: %w", msg, err))
	}

	merged, err := os.ReadFile(r.Paths.MergedSNMPFile())
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			measurementDevice.UpdateStatusCondition(chantico.ConditionExporterReload, metav1.ConditionUnknown, chantico.ReasonPending, "Merged SNMP file does not exist yet.")
			return steps.Continue()
		}
		return fail(err, "read merged SNMP file")
	}

	desiredHash := snmp.Hash(merged)

	exporter, err := r.getSnmpExporterDeployment(ctx)
	if err != nil {
		return fail(err, fmt.Sprintf("Error while retrieving SNMP exporter deployment %s", err))
	}

	current := exporter.Spec.Template.Annotations[md.ConfigHashAnnotation]
	if current == desiredHash {
		measurementDevice.UpdateStatusCondition(chantico.ConditionExporterReload, metav1.ConditionTrue, chantico.ReasonSucceeded, "SNMP exporter is up to date with merged config.")
		return steps.Continue()
	}

	patch := client.MergeFrom(exporter.DeepCopy())
	if exporter.Spec.Template.Annotations == nil {
		exporter.Spec.Template.Annotations = map[string]string{}
	}
	exporter.Spec.Template.Annotations[md.ConfigHashAnnotation] = desiredHash
	if err := r.Patch(ctx, exporter, patch); err != nil {
		return fail(err, fmt.Sprintf("patch deployment %s", err))
	}

	l.Info("Triggered SNMP exporter reload", "hash", desiredHash)
	measurementDevice.UpdateStatusCondition(chantico.ConditionExporterReload, metav1.ConditionTrue, chantico.ReasonSynced, "SNMP exporter deployment annotation updated to trigger reload.")
	return steps.Continue()
}

func (r *MeasurementDeviceReconciler) getSnmpExporterDeployment(ctx context.Context) (*appsv1.Deployment, error) {
	var deploy appsv1.Deployment
	if err := r.Get(ctx, client.ObjectKey{Name: "chantico-snmp", Namespace: "chantico"}, &deploy); err != nil {
		return nil, err
	}
	return &deploy, nil
}

func (r *MeasurementDeviceReconciler) setObservedGeneration(ctx context.Context, measurementDevice *chantico.MeasurementDevice) steps.StepResult {
	measurementDevice.Status.ObservedGeneration = measurementDevice.Generation
	return steps.Continue()
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
