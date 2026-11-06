package k8s

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// ApplyWithOwner makes the object in the cluster match desired: it creates the
// object if it does not exist, otherwise it updates the existing one. The
// caller's copySpec decides how the desired spec is written onto the existing
// object.
//
// While doing so it merges desired's labels into the existing ones and marks
// owner as the controller. These are applied inside the mutate callback
// because CreateOrUpdate overwrites the whole object with the server copy
// before mutating it, so anything set beforehand would be lost.
func ApplyWithOwner[T client.Object](
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	owner client.Object,
	desired T,
	copySpec func(existing, desired T),
) error {
	obj := desired.DeepCopyObject().(T)

	_, err := controllerutil.CreateOrUpdate(ctx, c, obj, func() error {
		if err := controllerutil.SetControllerReference(owner, obj, scheme); err != nil {
			return fmt.Errorf("set controller reference: %w", err)
		}

		labels := obj.GetLabels()
		if labels == nil {
			labels = make(map[string]string)
		}
		for key, value := range desired.GetLabels() {
			labels[key] = value
		}
		obj.SetLabels(labels)

		copySpec(obj, desired)
		return nil
	})
	return err
}
