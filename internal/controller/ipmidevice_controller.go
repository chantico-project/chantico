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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	chantico "chantico/api/v1alpha1"
	chanticov1alpha1 "chantico/api/v1alpha1"
	id "chantico/internal/ipmidevice"
	ph "chantico/internal/patch"
)

// IPMIDeviceReconciler reconciles a IPMIDevice object
type IPMIDeviceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=chantico.ci.tno.nl,resources=ipmidevices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=chantico.ci.tno.nl,resources=ipmidevices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=chantico.ci.tno.nl,resources=ipmidevices/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the IPMIDevice object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile
func (r *IPMIDeviceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Get the information needed to determine the state of the IPMIDevice
	ipmiDevice := &chantico.IPMIDevice{}
	err := r.Get(ctx, req.NamespacedName, ipmiDevice)
	if err != nil {
		return ctrl.Result{}, nil
	}

	log.Printf("Updating state of IPMI device %s\n", ipmiDevice.Name)
	patch := ph.Initialize(ctx, r.Client, ipmiDevice)
	id.UpdateState(ipmiDevice)
	patch.PatchStatus()

	result := id.StateMachine.ExecuteActions(ctx, r.Client, ipmiDevice, patch)
	if result != nil && result.Result != nil {
		return *result.Result, nil
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *IPMIDeviceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&chanticov1alpha1.IPMIDevice{}).
		Owns(&corev1.Secret{}).
		Complete(r)
}
