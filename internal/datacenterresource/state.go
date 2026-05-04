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

package datacenterresource

import (
	chantico "chantico/api/v1alpha1"
	"fmt"
	"slices"
)

const (
	StateInit             = "Init"
	StateEntry            = "Entry point"
	StateValidationFailed = "Validation Failed"
	StateDelete           = "Delete"
	StateEnd              = "End point"
)

func UpdateState(
	dataCenterResource *chantico.DataCenterResource,
) {
	// Covers the initialization pathological cases
	if dataCenterResource == nil {
		return
	}
	if dataCenterResource.Status.UpdateGeneration == 0 {
		dataCenterResource.Status.UpdateGeneration = 1
	}

	state := dataCenterResource.Status.State
	if !slices.Contains(dataCenterResource.ObjectMeta.Finalizers, chantico.DataCenterResourceGraphFinalizer) {
		dataCenterResource.Status.State = StateInit
	}

	// Covers lifecycle related changes
	switch {
	case dataCenterResource.Status.UpdateGeneration < dataCenterResource.ObjectMeta.Generation:
		dataCenterResource.Status.State = StateEntry
	case dataCenterResource.ObjectMeta.GetDeletionTimestamp() != nil:
		dataCenterResource.Status.State = StateDelete
	}

	// Realize the update
	switch state {
	case "", StateInit:
		dataCenterResource.Status.State = StateInit
		dataCenterResource.Status.UpdateGeneration = dataCenterResource.ObjectMeta.Generation
		return
	case StateEntry:
		dataCenterResource.Status.UpdateGeneration = dataCenterResource.ObjectMeta.Generation
		return
	case StateEnd, StateValidationFailed, StateDelete:
		return
	default:
		SetValidationError(dataCenterResource, fmt.Errorf("unknown state"), "")
		return
	}
}

func SetValidationError(
	dataCenterResource *chantico.DataCenterResource,
	err error,
	involvedResource string,
) {
	dataCenterResource.Status.State = StateValidationFailed
	dataCenterResource.Status.ErrorMessage = fmt.Sprintf("validation error: %s", err)
	dataCenterResource.Status.ErrorType = fmt.Sprintf("%T", err)
	dataCenterResource.Status.InvolvedResource = involvedResource
}

func ClearValidationError(
	dataCenterResource *chantico.DataCenterResource,
) {
	dataCenterResource.Status.State = StateInit
	dataCenterResource.Status.ErrorMessage = ""
	dataCenterResource.Status.ErrorType = ""
	dataCenterResource.Status.InvolvedResource = ""
}
