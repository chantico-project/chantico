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
	"strconv"

	chantico "chantico/api/v1alpha1"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"

	"chantico/internal/filestore"
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
	Scheme    *runtime.Scheme
	Namespace string
	Filestore filestore.FileStore
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
	r.Namespace = measurementDevice.GetNamespace()
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
		r.reconcileGeneratorFile,
		r.reconcileSNMPGeneratorJob,
		r.reconcileSNMPFileContent,
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

	log.FromContext(ctx).Info("Deleting MeasurementDevice files", "MeasurementDevice", measurementDevice.Name)
	jobs, err := r.getOwnedJobs(ctx, measurementDevice)
	if err != nil {
		return failed(measurementDevice, chantico.ConditionApplied, chantico.ReasonCleanupFailed, "Failed to get owned jobs", err)
	}
	for i := range jobs {
		job := &jobs[i]
		if err := r.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground)); client.IgnoreNotFound(err) != nil {
			return failed(measurementDevice, chantico.ConditionApplied, chantico.ReasonCleanupFailed, "Failed to delete owned job", err)
		}
	}

	filesToRemove := []string{
		md.GeneratorFile(measurementDevice.GetUID()),
		md.SnmpFile(measurementDevice.GetUID()),
	}
	for _, path := range filesToRemove {
		if err := r.Filestore.Remove(ctx, path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return failed(measurementDevice, chantico.ConditionApplied, chantico.ReasonCleanupFailed, "Error while removing SNMP file", err)
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
	path := md.GeneratorFile(measurementDevice.GetUID())

	observed, err := r.Filestore.ReadAll(ctx, path)
	if err != nil {
		// If the generator file does not exist yet, create it with the desired content.
		if errors.Is(err, fs.ErrNotExist) {
			desired, derr := desiredGeneratorConfig(measurementDevice)
			if derr != nil {
				return failed(measurementDevice, chantico.ConditionGenerated, chantico.ReasonGenerationFailed, "Failed to marshal generator config", derr)
			}
			if werr := r.Filestore.Write(ctx, path, bytes.NewReader(desired)); werr != nil {
				return failed(measurementDevice, chantico.ConditionGenerated, chantico.ReasonGenerationFailed, "Failed to write generator file "+path, werr)
			}
			log.FromContext(ctx).Info("Generator file has been generated successfully.", "path", path)
			return reconciled(measurementDevice, chantico.ConditionGenerated, "Generator file has been generated successfully.")
		}
		return failed(measurementDevice, chantico.ConditionGenerated, chantico.ReasonGenerationFailed, "Failed to read generator file "+path, err)
	}

	desired, err := desiredGeneratorConfig(measurementDevice)
	if err != nil {
		return failed(measurementDevice, chantico.ConditionGenerated, chantico.ReasonGenerationFailed, "Failed to marshal generator config", err)
	}

	if bytes.Equal(observed, desired) {
		return reconciled(measurementDevice, chantico.ConditionGenerated, "Generator file is up to date")
	}

	if err := r.Filestore.Write(ctx, path, bytes.NewReader(desired)); err != nil {
		return failed(measurementDevice, chantico.ConditionGenerated, chantico.ReasonGenerationFailed, "Failed to write generator file "+path, err)
	}

	log.FromContext(ctx).Info("Generator file has been generated successfully.", "path", path)
	return reconciled(measurementDevice, chantico.ConditionGenerated, "Generator file has been generated successfully")
}

func desiredGeneratorConfig(measurementDevice *chantico.MeasurementDevice) ([]byte, error) {
	return yaml.Marshal(snmp.GeneratorConfig{
		Auths:   map[string]*snmp.GeneratorAuth{measurementDevice.Name: &measurementDevice.Spec.Auth},
		Modules: map[string]*snmp.GeneratorModule{measurementDevice.Name: {Walk: measurementDevice.Spec.Walks}},
	})
}

func (r *MeasurementDeviceReconciler) reconcileSNMPGeneratorJob(ctx context.Context, measurementDevice *chantico.MeasurementDevice) steps.StepResult {
	jobs, err := r.getOwnedJobs(ctx, measurementDevice)
	if err != nil {
		return failed(measurementDevice, chantico.ConditionGenerated, chantico.ReasonGenerationFailed, "Failed to get owned SNMP Generator jobs", err)
	}

	switch len(jobs) {
	case 0:
		return r.createGeneratorJob(ctx, measurementDevice)
	case 1:
		return r.evaluateGeneratorJob(ctx, measurementDevice, &jobs[0])
	default:
		return failed(measurementDevice, chantico.ConditionGenerated, chantico.ReasonGenerationFailed, "Too many SNMP Generator jobs",
			fmt.Errorf("expected at most 1 owned job, found %d", len(jobs)))
	}
}

func (r *MeasurementDeviceReconciler) createGeneratorJob(
	ctx context.Context, measurementDevice *chantico.MeasurementDevice,
) steps.StepResult {
	job, err := md.BuildGeneratorJob(measurementDevice)
	if err != nil {
		return failed(measurementDevice, chantico.ConditionGenerated, chantico.ReasonGenerationFailed, "Failed to build SNMP Generator job", err)
	}
	if err := util.SetControllerReference(measurementDevice, job, r.Scheme); err != nil {
		return failed(measurementDevice, chantico.ConditionGenerated, chantico.ReasonGenerationFailed, "Failed to set controller reference for SNMP Generator job", err)
	}
	if err := r.Create(ctx, job); err != nil {
		return failed(measurementDevice, chantico.ConditionGenerated, chantico.ReasonGenerationFailed, "Failed to create SNMP Generator job", err)
	}

	log.FromContext(ctx).Info("Created SNMP Generator job", "job", job.Name)
	return waiting(measurementDevice, chantico.ConditionGenerated, chantico.ReasonGenerationPending, "SNMP Generator Job created")
}

func (r *MeasurementDeviceReconciler) evaluateGeneratorJob(ctx context.Context, measurementDevice *chantico.MeasurementDevice, job *batchv1.Job) steps.StepResult {
	l := log.FromContext(ctx)

	if jobGeneration(job) != measurementDevice.GetGeneration() {
		l.Info("Stale SNMP Generator job, deleting...", "job", job.Name)
		if err := r.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground)); client.IgnoreNotFound(err) != nil {
			return failed(measurementDevice, chantico.ConditionGenerated, chantico.ReasonGenerationFailed, "Failed to delete stale SNMP Generator job "+job.Name, err)
		}
		return steps.Stop()
	}

	switch {
	case isJobSuccessful(job):
		l.Info("Generator job succeeded", "job", job.Name)
		return progressing(measurementDevice, chantico.ConditionGenerated, chantico.ReasonGenerationPending, "SNMP Generator Job succeeded; generated config is being verified")
	case isJobFailed(job):
		l.Info("Generator job failed", "job", job.Name)
		return halted(measurementDevice, chantico.ConditionGenerated, chantico.ReasonGenerationFailed, "SNMP Generator Job failed")
	default:
		l.Info("Generator job is running", "job", job.Name)
		return waiting(measurementDevice, chantico.ConditionGenerated, chantico.ReasonGenerationPending, "SNMP Generator Job is running")
	}
}

func (r *MeasurementDeviceReconciler) reconcileSNMPFileContent(ctx context.Context, measurementDevice *chantico.MeasurementDevice) steps.StepResult {
	path := md.SnmpFile(measurementDevice.GetUID())
	config, err := r.Filestore.ReadAll(context.Background(), path)
	if err != nil {
		return failed(measurementDevice, chantico.ConditionGenerated, chantico.ReasonGenerationFailed, "Failed to read SNMP file "+path, err)
	}

	configSha := sha256.Sum256(config)
	configHash := hex.EncodeToString(configSha[:])

	if measurementDevice.Status.ConfigHash == configHash {
		return reconciled(measurementDevice, chantico.ConditionGenerated, "ConfigHash matches with SNMP configuration")
	}

	measurementDevice.Status.ConfigHash = configHash
	return reconciled(measurementDevice, chantico.ConditionGenerated, "ConfigHash has been updated to match with SNMP configuration")
}

func (r *MeasurementDeviceReconciler) reconcileMergedSNMPFile(ctx context.Context, measurementDevice *chantico.MeasurementDevice) steps.StepResult {
	merged, err := snmp.GetMergedSortedSNMPConfig(r.Filestore, md.SnmpSubDir)
	if err != nil {
		return failed(measurementDevice, chantico.ConditionApplied, chantico.ReasonApplyFailed, "Failed to read SNMP configs", err)
	}

	path := md.SnmpMergedFile
	existing, err := r.Filestore.ReadAll(context.Background(), path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return failed(measurementDevice, chantico.ConditionApplied, chantico.ReasonApplyFailed, "Failed to read merged SNMP file", err)
	}
	if bytes.Equal(existing, merged) {
		return progressing(measurementDevice, chantico.ConditionApplied, chantico.ReasonReconciling, "Merged SNMP file is up to date; exporter reload is being verified")
	}

	if err := r.Filestore.Write(ctx, path, bytes.NewReader(merged)); err != nil {
		return failed(measurementDevice, chantico.ConditionApplied, chantico.ReasonApplyFailed, "Failed to write merged SNMP file "+path, err)
	}

	return progressing(measurementDevice, chantico.ConditionApplied, chantico.ReasonReconciling, "Merged SNMP file has been written successfully; exporter reload is pending")
}

func (r *MeasurementDeviceReconciler) reconcileExporterReload(ctx context.Context, measurementDevice *chantico.MeasurementDevice) steps.StepResult {
	merged, err := r.Filestore.ReadAll(ctx, md.SnmpMergedFile)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return progressing(measurementDevice, chantico.ConditionApplied, chantico.ReasonGenerationPending, "Merged SNMP file does not exist yet")
		}
		return failed(measurementDevice, chantico.ConditionApplied, chantico.ReasonApplyFailed, "Failed to read merged SNMP file", err)
	}

	desiredHash := snmp.Hash(merged)

	exporter, err := r.getSnmpExporterDeployment(ctx)
	if err != nil {
		return failed(measurementDevice, chantico.ConditionApplied, chantico.ReasonDependencyUnavailable, "Failed to get SNMP exporter deployment", err)
	}

	current := exporter.Spec.Template.Annotations[md.ConfigHashAnnotation]
	if current == desiredHash {
		return reconciled(measurementDevice, chantico.ConditionApplied, "SNMP exporter is up to date with merged config.")
	}

	patch := client.MergeFrom(exporter.DeepCopy())
	if exporter.Spec.Template.Annotations == nil {
		exporter.Spec.Template.Annotations = map[string]string{}
	}
	exporter.Spec.Template.Annotations[md.ConfigHashAnnotation] = desiredHash
	if err := r.Patch(ctx, exporter, patch); err != nil {
		return failed(measurementDevice, chantico.ConditionApplied, chantico.ReasonApplyFailed, "Failed to patch SNMP exporter deployment "+exporter.Name, err)
	}

	log.FromContext(ctx).Info("Triggered SNMP exporter reload", "hash", desiredHash)
	return reconciled(measurementDevice, chantico.ConditionApplied, "SNMP exporter deployment annotation updated to trigger reload")
}

func (r *MeasurementDeviceReconciler) reconcileReady(ctx context.Context, measurementDevice *chantico.MeasurementDevice) steps.StepResult {
	return reconciled(measurementDevice, chantico.ConditionReady, "Fully reconciled and ready")
}

func (r *MeasurementDeviceReconciler) getSnmpExporterDeployment(ctx context.Context) (*appsv1.Deployment, error) {
	var deploy appsv1.Deployment
	if err := r.Get(ctx, client.ObjectKey{Name: "chantico-snmp", Namespace: r.Namespace}, &deploy); err != nil {
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
