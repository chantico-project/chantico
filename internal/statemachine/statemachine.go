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

package statemachine

import (
	"context"
	"slices"
	"strings"

	ph "chantico/internal/patch"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Reconcilable is the interface that CRD types must satisfy to be used with
// the generic state-action machine.
type Reconcilable interface {
	client.Object
	GetState() string
	SetState(string)
	GetUpdateGeneration() int64
	SetUpdateGeneration(int64)
	GetFinalizerName() string
	GetErrorMessage() string
	SetErrorMessage(string)
}

// ActionFunctionType distinguishes between Pure and IO action functions.
type ActionFunctionType int

// In this context, "Pure" means "does not modify kubernetes cluster resources"
const (
	ActionFunctionIO ActionFunctionType = iota
	ActionFunctionPure
)

// ActionResult carries the reconcile result and what kind of patch to apply.
type ActionResult struct {
	*ctrl.Result
	ph.PatchType
}

// ActionFunction represents a single step in the action pipeline for a state.
// Either Pure or IO must be set (not both), matching the Type field.
type ActionFunction[T Reconcilable] struct {
	Type ActionFunctionType
	Pure func(context.Context, T) *ActionResult
	IO   func(context.Context, client.Client, T) *ActionResult
}

// Machine is a generic state-action machine parameterized over a Reconcilable
// resource type. It maps states to ordered slices of action functions and
// knows which state represents failure (to break out of the action loop).
type Machine[T Reconcilable] struct {
	Actions   map[string][]ActionFunction[T]
	FailState string
}

// ExecuteActions runs the action functions registered for the resource's
// current state. After each action that returns a non-nil result, it patches
// the resource. It breaks early if the resource enters the FailState or if
// an action returns a non-nil ctrl.Result (indicating requeue).
func (m *Machine[T]) ExecuteActions(
	ctx context.Context,
	kubernetesClient client.Client,
	resource T,
	patch *ph.PatchHelper,
) *ActionResult {
	var result *ActionResult = nil
	actionFunctions := m.Actions[resource.GetState()]
	for _, actionFunction := range actionFunctions {
		switch actionFunction.Type {
		case ActionFunctionPure:
			result = actionFunction.Pure(ctx, resource)
		case ActionFunctionIO:
			result = actionFunction.IO(ctx, kubernetesClient, resource)
		}

		if result != nil {
			patch.Patch(result.PatchType)
			if result.Result != nil || resource.GetState() == m.FailState {
				break
			}
		}
	}
	return result
}

// InitializeFinalizer is a generic Pure action that adds the resource's
// finalizer if it is not already present.
func InitializeFinalizer[T Reconcilable](ctx context.Context, resource T) *ActionResult {
	l := log.FromContext(ctx)

	if slices.Contains(resource.GetFinalizers(), resource.GetFinalizerName()) {
		return nil
	}
	resource.SetFinalizers(append(resource.GetFinalizers(), resource.GetFinalizerName()))
	l.Info("Added finalizer", "finalizers", strings.Join(resource.GetFinalizers(), ", "))
	return &ActionResult{PatchType: ph.PatchResource}
}

// RemoveFinalizer is a generic Pure action that removes the resource's
// finalizer when the resource is being deleted.
func RemoveFinalizer[T Reconcilable](ctx context.Context, resource T) *ActionResult {
	if resource.GetDeletionTimestamp().IsZero() {
		return nil
	}
	finalizers := resource.GetFinalizers()
	filtered := slices.DeleteFunc(slices.Clone(finalizers), func(f string) bool {
		return f == resource.GetFinalizerName()
	})
	resource.SetFinalizers(filtered)
	return &ActionResult{PatchType: ph.PatchResource}
}
