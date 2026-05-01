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
	"strings"

	chantico "chantico/api/v1alpha1"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"

	config "chantico/internal/config"
	"chantico/internal/snmp"
	"chantico/internal/snmpgenerator"
	"chantico/internal/steps"
	"crypto/sha256"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlcontroller "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	util "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	yaml "sigs.k8s.io/yaml"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:rbac:groups=chantico.ci.tno.nl,resources=snmpdevices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=chantico.ci.tno.nl,resources=snmpdevices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=chantico.ci.tno.nl,resources=snmpdevices/finalizers,verbs=create;update;patch

// SnmpGeneratorReconciler reconciles a SNMP generator
type SnmpGeneratorReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Config config.Config
	Paths  snmpgenerator.Paths
}

func (r *SnmpGeneratorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&chantico.SNMPDevice{}).
		Owns(&batchv1.Job{}).
		WithOptions(ctrlcontroller.Options{MaxConcurrentReconciles: 1}). // Race conditions are possible if multiple generator jobs run at the same time, so we only allow one at a time.
		Complete(r)
}

func (r *SnmpGeneratorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	// Get the SNMP device
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
		if err := r.reconcileStatus(snmpDevice); err != nil {
			reterr = errors.Join(reterr, err)
		}
		if err := helper.Patch(ctx, snmpDevice); err != nil {
			reterr = errors.Join(reterr, err)
		}
	}()

	return steps.Run(ctx, snmpDevice,
		r.reconcileDeletion,
		r.ensureFinalizerIsSet,
		r.reconcileGeneratorFile,
		r.reconcileMibFile,
		r.ensureSNMPFileExists,
		r.reconcileSNMPGeneratorJob,
		r.reconcileSNMPFileContent,
		r.reconcileMergedSNMPFile,
		r.reconcileExporterReload,
		r.setObservedGeneration,
	)
}

func (r *SnmpGeneratorReconciler) reconcileStatus(snmpDevice *chantico.SNMPDevice) error {
	// should use ObservedGeneration for determining up-to-date or old conditions?
	// we should probably also use a global ObservedGeneration (so then we can see what reconcile has been, and whether it matches the conditions)
	jobCondition := meta.FindStatusCondition(snmpDevice.Status.Conditions, string(chantico.ConditionJob))
	if jobCondition == nil {
		snmpDevice.UpdateStatusCondition(chantico.ConditionJob, metav1.ConditionUnknown, chantico.ReasonPending, "Job condition is pending")
		return nil
	}

	snmpDevice.UpdateStatusJobCondition(jobCondition)
	return nil
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
			return steps.Error(fmt.Errorf("remove %s: %w", path, err))
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

func desiredGeneratorConfig(md *chantico.SNMPDevice) ([]byte, error) {
	return yaml.Marshal(snmp.GeneratorConfig{
		Auths:   map[string]*snmp.GeneratorAuth{md.Name: &md.Spec.Auth},
		Modules: map[string]*snmp.GeneratorModule{md.Name: {Walk: md.Spec.Walks}},
	})
}

func (r *SnmpGeneratorReconciler) reconcileGeneratorFile(ctx context.Context, snmpDevice *chantico.SNMPDevice) steps.StepResult {
	path := r.Paths.GeneratorFile(snmpDevice.GetUID())

	observed, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return steps.Error(fmt.Errorf("read generator file %s: %w", path, err))
	}

	desired, err := desiredGeneratorConfig(snmpDevice)
	if err != nil {
		snmpDevice.UpdateStatusCondition(chantico.ConditionGeneratorFile, metav1.ConditionFalse, chantico.ReasonFailed, fmt.Sprintf("failed to marshal generator config: %v", err))
		return steps.Error(err)
	}

	if bytes.Equal(observed, desired) {
		snmpDevice.UpdateStatusCondition(chantico.ConditionGeneratorFile, metav1.ConditionTrue, chantico.ReasonFileWritten, "Generator file is up to date.")
		return steps.Continue()
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0777); err != nil {
		return steps.Error(err)
	}

	if err := os.WriteFile(path, desired, 0777); err != nil {
		snmpDevice.UpdateStatusCondition(chantico.ConditionGeneratorFile, metav1.ConditionFalse, chantico.ReasonFailed, fmt.Sprintf("failed to write generator file: %v", err))
		return steps.Error(fmt.Errorf("write generator file %s: %w", path, err))
	}

	snmpDevice.UpdateStatusCondition(chantico.ConditionGeneratorFile, metav1.ConditionTrue, chantico.ReasonFileWritten, "Generator file has been generated successfully.")
	return steps.Continue()
}

func (r *SnmpGeneratorReconciler) reconcileMibFile(ctx context.Context, snmpDevice *chantico.SNMPDevice) steps.StepResult {
	// TODO: Check existence of MIB file.
	return steps.Continue()
}

func (r *SnmpGeneratorReconciler) ensureSNMPFileExists(ctx context.Context, snmpDevice *chantico.SNMPDevice) steps.StepResult {
	path := r.Paths.SNMPFile(snmpDevice.GetUID())

	_, err := os.ReadFile(path)
	if err == nil {
		// file exists, awesome
		return steps.Continue()
	}
	if !errors.Is(err, fs.ErrNotExist) {
		// another error, maybe permissions, or smth
		return steps.Error(err)
	}

	// create file
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0777); err != nil {
		return steps.Error(err)
	}
	err = os.WriteFile(path, []byte{}, 0777)
	if err != nil {
		return steps.Error(err)
	}
	return steps.Continue()
}

func (r *SnmpGeneratorReconciler) reconcileSNMPGeneratorJob(ctx context.Context, snmpDevice *chantico.SNMPDevice) steps.StepResult {
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
	job, err := snmpgenerator.BuildGeneratorJob(r.Config.Images.SnmpGenerator, snmpDevice)
	if err != nil {
		return steps.Error(err)
	}
	if err := controllerutil.SetControllerReference(snmpDevice, job, r.Scheme); err != nil {
		return steps.Error(err)
	}
	if err := r.Create(ctx, job); err != nil {
		return steps.Error(fmt.Errorf("create generator job: %w", err))
	}
	snmpDevice.UpdateStatusCondition(chantico.ConditionJob, metav1.ConditionUnknown,
		chantico.ReasonJobPending, "SNMP Generator Job created")
	return steps.Stop() // wait for the watch to wake us
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
		snmpDevice.UpdateStatusCondition(chantico.ConditionJob, metav1.ConditionTrue,
			chantico.ReasonJobSucceeded, "SNMP Generator Job succeeded")
		return steps.Continue()
	case isJobFailed(job):
		snmpDevice.UpdateStatusCondition(chantico.ConditionJob, metav1.ConditionFalse,
			chantico.ReasonJobFailed, "SNMP Generator Job failed")
		return steps.Stop()
	default:
		snmpDevice.UpdateStatusCondition(chantico.ConditionJob, metav1.ConditionUnknown,
			chantico.ReasonJobPending, "SNMP Generator Job is running")
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

func (r *SnmpGeneratorReconciler) reconcileMergedSNMPFile(ctx context.Context, d *chantico.SNMPDevice) steps.StepResult {
	fragments, err := r.readPerDeviceFragments()
	if err != nil {
		return steps.Error(err)
	}

	merged, err := snmp.Merge(snmp.SortedFragments(fragments))
	if err != nil {
		return steps.Error(fmt.Errorf("merge per-device snmp configs: %w", err))
	}

	path := r.Paths.MergedSNMPFile()
	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return steps.Error(fmt.Errorf("read %s: %w", path, err))
	}
	if bytes.Equal(existing, merged) {
		return steps.Continue()
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return steps.Error(err)
	}
	// Atomic write so the exporter never sees a truncated file.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, merged, 0o644); err != nil {
		return steps.Error(fmt.Errorf("write %s: %w", tmp, err))
	}
	if err := os.Rename(tmp, path); err != nil {
		return steps.Error(fmt.Errorf("rename %s -> %s: %w", tmp, path, err))
	}
	return steps.Continue()
}

func (r *SnmpGeneratorReconciler) reconcileExporterReload(ctx context.Context, d *chantico.SNMPDevice) steps.StepResult {
	merged, err := os.ReadFile(r.Paths.MergedSNMPFile())
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return steps.Continue() // nothing to reload yet
		}
		return steps.Error(err)
	}
	desiredHash := snmp.Hash(merged)

	var deploy appsv1.Deployment
	if err := r.Get(ctx, r.Config.SnmpDeployment, &deploy); err != nil {
		if apierrors.IsNotFound(err) {
			// Helm chart not installed (e.g. envtest). Don't fail reconcile.
			return steps.Continue()
		}
		return steps.Error(err)
	}

	current := deploy.Spec.Template.Annotations[snmpgenerator.ConfigHashAnnotation]
	if current == desiredHash {
		return steps.Continue()
	}

	patch := client.MergeFrom(deploy.DeepCopy())
	if deploy.Spec.Template.Annotations == nil {
		deploy.Spec.Template.Annotations = map[string]string{}
	}
	deploy.Spec.Template.Annotations[snmpgenerator.ConfigHashAnnotation] = desiredHash
	if err := r.Patch(ctx, &deploy, patch); err != nil {
		return steps.Error(fmt.Errorf("patch deployment %s: %w", r.Config.SnmpDeployment, err))
	}
	return steps.Continue()
}

// readPerDeviceFragments reads every snmp-*.yaml in the SNMP dir and
// returns them keyed by filename so SortedFragments can produce a
// deterministic merge order.
func (r *SnmpGeneratorReconciler) readPerDeviceFragments() (map[string][]byte, error) {
	dir := r.Paths.SNMPDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("list %s: %w", dir, err)
	}
	out := map[string][]byte{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		// Only per-device files: snmp-<uid>.yaml. Excludes the merged snmp.yml.
		if !strings.HasPrefix(name, "snmp-") || filepath.Ext(name) != ".yaml" {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		if len(bytes.TrimSpace(content)) == 0 {
			continue // empty placeholder before generator job runs
		}
		out[name] = content
	}
	return out, nil
}

func (r *SnmpGeneratorReconciler) setObservedGeneration(ctx context.Context, snmpDevice *chantico.SNMPDevice) steps.StepResult {
	snmpDevice.Status.ObservedGeneration = snmpDevice.Generation
	return steps.Continue()
}

func jobGeneration(job *batchv1.Job) int64 {
	s := job.GetAnnotations()[snmpgenerator.GenerationAnnotation]
	n, _ := strconv.ParseInt(s, 10, 64)
	return n // 0 if missing/invalid → guaranteed to mismatch any real generation
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

	// TODO: this can be optimized with indexing (at the manager)
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
