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
	"log"

	// "log"

	chantico "chantico/api/v1alpha1"
	// md "chantico/internal/measurementdevice"

	batchv1 "k8s.io/api/batch/v1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	// ph "chantico/internal/patch"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MeasurementDeviceReconciler reconciles a MeasurementDevice object
type MeasurementDeviceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=chantico.ci.tno.nl,resources=measurementdevices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=chantico.ci.tno.nl,resources=measurementdevices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=chantico.ci.tno.nl,resources=measurementdevices/finalizers,verbs=create;update;patch

var counter = 0

func (r *MeasurementDeviceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	counter = counter + 1
	log.Println(req.NamespacedName, counter)

	md := &chantico.MeasurementDevice{}
	err := r.Get(ctx, req.NamespacedName, md)
	if err != nil {
		log.Println("error getting object", err)
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if res := r.reconcileFinalizer(ctx, md); res.Stop {
		log.Println("stopping finalizer reconcile", res.Error)
		return res.Result, res.Error
	}

	if res := r.reconcileJob(ctx, md); res.Stop {
		log.Println("stopping job reconcile", res.Error)
		return res.Result, res.Error
	}

	log.Println("all good")

	return ctrl.Result{}, nil

	

	// cachedMeasurementDevice := &chantico.MeasurementDevice{}
	// err := r.Get(ctx, req.NamespacedName, cachedMeasurementDevice)
	// if err != nil {
	// 	log.Println(err)
	// 	return ctrl.Result{}, nil
	// }

	// measurementDevice := cachedMeasurementDevice.DeepCopy()
	// log.Println("jobname", measurementDevice.Status.JobName)

	// measurementDevice.Status.JobName = strconv.Itoa(counter)

	// patch := client.MergeFrom(cachedMeasurementDevice)
	// r.Status().Patch(ctx, measurementDevice, patch)
	// measurementDevice.Status.JobName = strconv.Itoa(counter)
	// r.Status().Patch(ctx, measurementDevice, patch)

	// meta.SetStatusCondition(&measurementDevice.Status.Conditions, metav1.Condition{
	// 	Type:               "Hello",
	// 	Status:             metav1.ConditionTrue,
	// 	Reason:             "AllPodsReady",
	// 	ObservedGeneration: measurementDevice.Generation,
	// })

	// meta.SetStatusCondition(&measurementDevice.Status.Conditions, metav1.Condition{
	// 	Type:               "Ready",
	// 	Status:             metav1.ConditionTrue,
	// 	Reason:             "AllPodsReady",
	// 	ObservedGeneration: measurementDevice.Generation,
	// })

	// fmt.Println(meta.IsStatusConditionTrue(measurementDevice.Status.Conditions, "Hello"))

	// patch := client.MergeFrom(cachedMeasurementDevice)
	// r.Status().Patch(ctx, measurementDevice, patch)

	// r.reconcileFinalizers() // return if it returns something
	// r.reconcileJob()
	// r.reconcile...() // how to write tests?

	// meta.IsStatusConditionTrue()

	// Get the information needed to determine the state of the MeasurementDevice
	// measurementDevice := &chantico.MeasurementDevice{}
	// err := r.Get(ctx, req.NamespacedName, measurementDevice)
	// if err != nil {
	// 	// IVO: I believe this return is a bit strange.
	// 	return ctrl.Result{}, nil
	// }

	// job := &batchv1.Job{}
	// _ = r.Get(ctx, client.ObjectKey{Name: measurementDevice.Status.JobName, Namespace: "chantico"}, job) // IVO: why not check if JobName exists?

	// log.Printf("Updating state of measurement device %s\n", measurementDevice.Name)
	// patch := ph.Initialize(ctx, r.Client, measurementDevice) // IVO: I don't get it. You give the measurement device to the patch, which has it as a copy and the object.
	// md.UpdateState(measurementDevice, job) // here we update the state of the measurement device and job
	// patch.PatchStatus() // since the measurment device is a pointer, we patch the status

	// result := md.ExecuteActions(ctx, r.Client, measurementDevice, patch)
	// if result != nil && result.Result != nil {
	// 	return *result.Result, nil
	// }

	// return ctrl.Result{}, nil
}

type ReconcileResult struct {
	Result ctrl.Result
	Error  error
	Stop   bool
}

func (r *MeasurementDeviceReconciler) reconcileDeletion() {
	// logic
}

func (r *MeasurementDeviceReconciler) reconcileFinalizer(ctx context.Context, md *chantico.MeasurementDevice) ReconcileResult {
	base := md.DeepCopy()

	addedFinalizer := controllerutil.AddFinalizer(md, chantico.MeasurementDeviceFinalizer)
	if !addedFinalizer {
		return ReconcileResult{Stop: false}
	}

	if err := r.Patch(ctx, md, client.MergeFrom(base)); err != nil {
		return ReconcileResult{Error: err, Stop: true}
	}

	return ReconcileResult{Stop: true}
}

func (r *MeasurementDeviceReconciler) reconcileJob(ctx context.Context, md *chantico.MeasurementDevice) ReconcileResult {
	jobList := &batchv1.JobList{}

	if err := r.List(ctx, jobList, client.InNamespace(md.GetNamespace())); err != nil {
		return ReconcileResult{Error: err, Stop: true}
	}

	var ownedJobs []batchv1.Job
	for _, job := range jobList.Items {
		for _, ownerRef := range job.OwnerReferences {
			if ownerRef.UID == md.GetUID() {
				ownedJobs = append(ownedJobs, job)
			}
		}
	}

	if len(ownedJobs) == 0 {
		job := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      md.GetName(),
				Namespace: md.GetNamespace(),
			},

			Spec: batchv1.JobSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "sleep",
								Image: "busybox",
								Command: []string{
									"sh", "-c", "echo Pod started; sleep 10; echo Pod finished",
								},
							},
						},
						RestartPolicy: corev1.RestartPolicyNever,
					},
				},
			},
		}
		if err := controllerutil.SetControllerReference(md, job, r.Scheme); err != nil {
			return ReconcileResult{Error: err, Stop: true}
		}

		if err := r.Create(ctx, job); err != nil {
			return ReconcileResult{Error: err, Stop: true}
		}

		log.Println("creating job")

		return ReconcileResult{Stop: true}
	}

	log.Println("job", ownedJobs[0].Generation, ownedJobs[0].Status.Conditions)

	return ReconcileResult{Stop: false}
}

func (r *MeasurementDeviceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&chantico.MeasurementDevice{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}
