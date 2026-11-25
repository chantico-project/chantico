/*
Copyright 2025-2026.

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
	"errors"

	"github.com/go-logr/logr"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Setup function for the controller logger.
// It returns a function that constructs a logger with the controller name and the resource name from the reconcile request.
func controllerLogConstructor(mgr ctrl.Manager, name string) func(*reconcile.Request) logr.Logger {
	return func(req *reconcile.Request) logr.Logger {
		log := mgr.GetLogger().WithName(name)
		if req != nil {
			log = log.WithValues("resource", req.Name)
		}
		return log
	}
}

// newStatusPatcher creates a deferred status patcher that updates the observed generation.
func newStatusPatcher(ctx context.Context, c client.Client, obj client.Object) (func(*error), error) {
	patcher, err := patch.NewHelper(obj, c)
	if err != nil {
		return nil, err
	}

	return func(reterr *error) {
		if patchErr := patcher.Patch(ctx, obj, patch.WithStatusObservedGeneration{}); patchErr != nil {
			if *reterr == nil {
				*reterr = patchErr
			} else {
				*reterr = errors.Join(*reterr, patchErr)
			}
		}
	}, nil
}
