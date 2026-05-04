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
	"log"
	"os"
	"path/filepath"
	"strconv"

	chantico "chantico/api/v1alpha1"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"

	"chantico/internal/snmp"
	"chantico/internal/snmpgenerator"
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

	yaml "go.yaml.in/yaml/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:rbac:groups=chantico.ci.tno.nl,resources=snmpdevices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=chantico.ci.tno.nl,resources=snmpdevices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=chantico.ci.tno.nl,resources=snmpdevices/finalizers,verbs=create;update;patch

// SnmpGeneratorReconciler reconciles a SNMP generator
type SnmpGeneratorReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Paths  snmpgenerator.Paths
}

func (r *SnmpGeneratorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&chantico.SNMPDevice{}).
		Owns(&batchv1.Job{}).
		WithOptions(ctrlcontroller.Options{MaxConcurrentReconciles: 1}). // Race conditions might occur when multiple generator jobs run simultaneously, so only allow one at a time.
		Complete(r)
}

func (r *SnmpGeneratorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	snmpDevice := &chantico.SNMPDevice{}
	err := r.Get(ctx, req.NamespacedName, snmpDevice)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Helper function makes a deep copy of SNMP device, and Patches the spec/status as needed at the end of reconcile function.
	helper, err := patch.NewHelper(snmpDevice, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}
	defer func() {
		if err := helper.Patch(ctx, snmpDevice); err != nil {
			reterr = errors.Join(reterr, err)
		}
	}()

	return steps.Run(ctx, snmpDevice,
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

func (r *SnmpGeneratorReconciler) reconcileDeletion(ctx context.Context, snmpDevice *chantico.SNMPDevice) steps.StepResult {
	if snmpDevice.ObjectMeta.GetDeletionTimestamp() == nil {
		return steps.Continue()
	}

	if !util.ContainsFinalizer(snmpDevice, chantico.SNMPDeviceFinalizer) {
		return steps.Stop()
	}

	jobs, err := r.getOwnedJobs(ctx, snmpDevice)
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
		r.Paths.GeneratorFile(snmpDevice.GetUID()),
		r.Paths.SNMPFile(snmpDevice.GetUID()),
	}
	for _, path := range filesToRemove {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return steps.Error(fmt.Errorf("Error while removing SNMP file %s: %w", path, err))
		}
	}

	if res := r.reconcileMergedSNMPFile(ctx, snmpDevice); res.Action == steps.ActionError {
		return res
	}
	if res := r.reconcileExporterReload(ctx, snmpDevice); res.Action == steps.ActionError {
		return res
	}

	util.RemoveFinalizer(snmpDevice, chantico.SNMPDeviceFinalizer)
	return steps.Stop()
}

func (r *SnmpGeneratorReconciler) ensureFinalizerIsSet(ctx context.Context, snmpDevice *chantico.SNMPDevice) steps.StepResult {
	if util.ContainsFinalizer(snmpDevice, chantico.SNMPDeviceFinalizer) {
		return steps.Continue()
	}
	util.AddFinalizer(snmpDevice, chantico.SNMPDeviceFinalizer)
	return steps.Stop()
}

func (r *SnmpGeneratorReconciler) reconcileGeneratorFile(ctx context.Context, snmpDevice *chantico.SNMPDevice) steps.StepResult {
	path := r.Paths.GeneratorFile(snmpDevice.GetUID())

	observed, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		log.Printf("Cannot read generator file %s: %v", path, err)
		return steps.Error(fmt.Errorf("read generator file %s: %w", path, err))
	}

	desired, err := desiredGeneratorConfig(snmpDevice)
	if err != nil {
		snmpDevice.UpdateStatusCondition(chantico.ConditionGeneratorFile, metav1.ConditionFalse, chantico.ReasonFailed, fmt.Sprintf("failed to marshal generator config: %v", err))
		return steps.Error(err)
	}

	if bytes.Equal(observed, desired) {
		snmpDevice.UpdateStatusCondition(chantico.ConditionGeneratorFile, metav1.ConditionTrue, chantico.ReasonSynced, "Generator file is up to date.")
		return steps.Continue()
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0777); err != nil {
		return steps.Error(err)
	}

	if err := os.WriteFile(path, desired, 0777); err != nil {
		log.Printf("Cannot write generator file %s: %v", path, err)
		snmpDevice.UpdateStatusCondition(chantico.ConditionGeneratorFile, metav1.ConditionFalse, chantico.ReasonFailed, fmt.Sprintf("failed to write generator file: %v", err))
		return steps.Error(fmt.Errorf("write generator file %s: %w", path, err))
	}

	snmpDevice.UpdateStatusCondition(chantico.ConditionGeneratorFile, metav1.ConditionTrue, chantico.ReasonFileWritten, "Generator file has been generated successfully.")
	return steps.Continue()
}

func desiredGeneratorConfig(snmpDevice *chantico.SNMPDevice) ([]byte, error) {
	return yaml.Marshal(snmp.GeneratorConfig{
		Auths:   map[string]*snmp.GeneratorAuth{snmpDevice.Name: &snmpDevice.Spec.Auth},
		Modules: map[string]*snmp.GeneratorModule{snmpDevice.Name: {Walk: snmpDevice.Spec.Walks}},
	})
}

func (r *SnmpGeneratorReconciler) reconcileSNMPGeneratorJob(ctx context.Context, snmpDevice *chantico.SNMPDevice) steps.StepResult {
	if err := os.MkdirAll(r.Paths.SNMPDir(), 0777); err != nil {
		return steps.Error(fmt.Errorf("Error while creating SNMP folder: %w", err))
	}

	jobs, err := r.getOwnedJobs(ctx, snmpDevice)
	if err != nil {
		return steps.Error(err)
	}

	switch len(jobs) {
	case 0:
		return r.createGeneratorJob(ctx, snmpDevice)
	case 1:
		return r.evaluateGeneratorJob(ctx, snmpDevice, &jobs[0])
	default:
		return steps.Error(fmt.Errorf("snmpdevice %s owns %d jobs, expected at most 1", snmpDevice.GetName(), len(jobs)))
	}
}

func (r *SnmpGeneratorReconciler) createGeneratorJob(
	ctx context.Context, snmpDevice *chantico.SNMPDevice,
) steps.StepResult {
	job, err := snmpgenerator.BuildGeneratorJob(snmpDevice)
	if err != nil {
		return steps.Error(err)
	}
	if err := controllerutil.SetControllerReference(snmpDevice, job, r.Scheme); err != nil {
		return steps.Error(err)
	}
	if err := r.Create(ctx, job); err != nil {
		return steps.Error(fmt.Errorf("create generator job: %w", err))
	}
	snmpDevice.UpdateStatusCondition(chantico.ConditionJob, metav1.ConditionUnknown, chantico.ReasonPending, "SNMP Generator Job created")
	return steps.Stop()
}

func (r *SnmpGeneratorReconciler) evaluateGeneratorJob(ctx context.Context, snmpDevice *chantico.SNMPDevice, job *batchv1.Job) steps.StepResult {
	if jobGeneration(job) != snmpDevice.GetGeneration() {
		// stale — delete and let the next reconcile recreate.
		if err := r.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground)); client.IgnoreNotFound(err) != nil {
			return steps.Error(fmt.Errorf("delete stale job: %w", err))
		}
		return steps.Stop()
	}

	switch {
	case isJobSuccessful(job):
		snmpDevice.UpdateStatusCondition(chantico.ConditionJob, metav1.ConditionTrue, chantico.ReasonSucceeded, "SNMP Generator Job succeeded")
		return steps.Continue()
	case isJobFailed(job):
		snmpDevice.UpdateStatusCondition(chantico.ConditionJob, metav1.ConditionFalse, chantico.ReasonFailed, "SNMP Generator Job failed")
		return steps.Stop()
	default:
		snmpDevice.UpdateStatusCondition(chantico.ConditionJob, metav1.ConditionUnknown, chantico.ReasonPending, "SNMP Generator Job is running")
		return steps.Stop()
	}
}

func (r *SnmpGeneratorReconciler) reconcileSNMPFileContent(ctx context.Context, snmpDevice *chantico.SNMPDevice) steps.StepResult {
	path := r.Paths.SNMPFile(snmpDevice.GetUID())
	config, err := os.ReadFile(path)
	if err != nil {
		return steps.Error(err)
	}

	configSha := sha256.Sum256(config)
	configHash := hex.EncodeToString(configSha[:])

	if snmpDevice.Status.ConfigHash == configHash {
		snmpDevice.UpdateStatusCondition(chantico.ConditionConfig, metav1.ConditionTrue, chantico.ReasonSucceeded, "ConfigHash matches with SNMP configuration")
		return steps.Continue()
	}

	snmpDevice.Status.ConfigHash = configHash
	snmpDevice.UpdateStatusCondition(chantico.ConditionConfig, metav1.ConditionTrue, chantico.ReasonSynced, "ConfigHash has been updated to match with SNMP configuration")
	return steps.Continue()
}

func (r *SnmpGeneratorReconciler) reconcileMergedSNMPFile(ctx context.Context, snmpDevice *chantico.SNMPDevice) steps.StepResult {
	fail := func(err error, msg string) steps.StepResult {
		log.Printf("%s: %v", msg, err)
		snmpDevice.UpdateStatusCondition(chantico.ConditionConfig, metav1.ConditionFalse, chantico.ReasonFailed, fmt.Sprintf("%s: %v", msg, err))
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
		snmpDevice.UpdateStatusCondition(chantico.ConditionConfig, metav1.ConditionTrue, chantico.ReasonSynced, "Merged SNMP file is up to date.")
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

	snmpDevice.UpdateStatusCondition(chantico.ConditionConfig, metav1.ConditionTrue, chantico.ReasonFileWritten, "Merged SNMP file has been written successfully.")
	return steps.Continue()
}

func (r *SnmpGeneratorReconciler) reconcileExporterReload(ctx context.Context, snmpDevice *chantico.SNMPDevice) steps.StepResult {
	fail := func(err error, msg string) steps.StepResult {
		log.Printf("%s: %v", msg, err)
		snmpDevice.UpdateStatusCondition(chantico.ConditionExporterReload, metav1.ConditionFalse, chantico.ReasonFailed, fmt.Sprintf("%s: %v", msg, err))
		return steps.Error(fmt.Errorf("%s: %w", msg, err))
	}

	merged, err := os.ReadFile(r.Paths.MergedSNMPFile())
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			snmpDevice.UpdateStatusCondition(chantico.ConditionExporterReload, metav1.ConditionUnknown, chantico.ReasonPending, "Merged SNMP file does not exist yet.")
			return steps.Continue()
		}
		return fail(err, "read merged SNMP file")
	}

	desiredHash := snmp.Hash(merged)

	exporter, err := r.getSnmpExporterDeployment(ctx)
	if err != nil {
		return fail(err, fmt.Sprintf("Error while retrieving SNMP exporter deployment %s", err))
	}

	current := exporter.Spec.Template.Annotations[snmpgenerator.ConfigHashAnnotation]
	if current == desiredHash {
		snmpDevice.UpdateStatusCondition(chantico.ConditionExporterReload, metav1.ConditionTrue, chantico.ReasonSucceeded, "SNMP exporter is up to date with merged config.")
		return steps.Continue()
	}

	patch := client.MergeFrom(exporter.DeepCopy())
	if exporter.Spec.Template.Annotations == nil {
		exporter.Spec.Template.Annotations = map[string]string{}
	}
	exporter.Spec.Template.Annotations[snmpgenerator.ConfigHashAnnotation] = desiredHash
	if err := r.Patch(ctx, exporter, patch); err != nil {
		return fail(err, fmt.Sprintf("patch deployment %s", err))
	}

	snmpDevice.UpdateStatusCondition(chantico.ConditionExporterReload, metav1.ConditionTrue, chantico.ReasonSynced, "SNMP exporter deployment annotation updated to trigger reload.")
	return steps.Continue()
}

func (r *SnmpGeneratorReconciler) getSnmpExporterDeployment(ctx context.Context) (*appsv1.Deployment, error) {
	var deploy appsv1.Deployment
	if err := r.Get(ctx, client.ObjectKey{Name: "chantico-snmp", Namespace: "chantico"}, &deploy); err != nil {
		return nil, err
	}
	return &deploy, nil
}

func (r *SnmpGeneratorReconciler) setObservedGeneration(ctx context.Context, snmpDevice *chantico.SNMPDevice) steps.StepResult {
	snmpDevice.Status.ObservedGeneration = snmpDevice.Generation
	return steps.Continue()
}

func jobGeneration(job *batchv1.Job) int64 {
	s := job.GetAnnotations()[snmpgenerator.GenerationAnnotation]
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

func (r *SnmpGeneratorReconciler) getOwnedJobs(ctx context.Context, snmpDevice *chantico.SNMPDevice) ([]batchv1.Job, error) {
	jobList := &batchv1.JobList{}
	if err := r.List(ctx, jobList, client.InNamespace(snmpDevice.GetNamespace())); err != nil {
		return nil, err
	}

	var ownedJobs []batchv1.Job
	for _, job := range jobList.Items {
		for _, ownerRef := range job.OwnerReferences {
			if ownerRef.UID == snmpDevice.GetUID() {
				ownedJobs = append(ownedJobs, job)
			}
		}
	}
	return ownedJobs, nil
}
