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
	sm "chantico/internal/statemachine"
)

var StateMachine = sm.Machine[*chantico.DataCenterResource]{
	Actions: map[string][]sm.ActionFunction[*chantico.DataCenterResource]{
		StateInit: {
			{Type: sm.ActionFunctionPure, Pure: sm.InitializeFinalizer[*chantico.DataCenterResource]},
		},
		StateEntry: {
			{Type: sm.ActionFunctionPure, Pure: WriteRuleFile},
		},
		StateDelete: {
			{Type: sm.ActionFunctionPure, Pure: DeleteRuleFile},
			{Type: sm.ActionFunctionPure, Pure: sm.RemoveFinalizer[*chantico.DataCenterResource]},
		},

		StateValidationFailed: {},
		StateEnd:              {},
	},
	FailState: StateValidationFailed,
}
