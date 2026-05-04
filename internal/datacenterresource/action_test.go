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
	ph "chantico/internal/patch"
	sm "chantico/internal/statemachine"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestInitializeFinalizer(t *testing.T) {
	testCases := map[string]struct {
		Case               *chantico.DataCenterResource
		ExpectedPatchType  ph.PatchType
		ExpectedNil        bool
		ExpectedFinalizers []string
	}{
		"empty finalizer": {
			Case: &chantico.DataCenterResource{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: []string{},
				}},
			ExpectedPatchType:  ph.PatchResource,
			ExpectedFinalizers: []string{chantico.DataCenterResourceGraphFinalizer},
		},
		"already initialized": {
			Case: &chantico.DataCenterResource{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: []string{"test"},
				}},
			ExpectedPatchType:  ph.PatchResource,
			ExpectedFinalizers: []string{"test", chantico.DataCenterResourceGraphFinalizer},
		},
		"already contains": {
			Case: &chantico.DataCenterResource{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: []string{chantico.DataCenterResourceGraphFinalizer},
				}},
			ExpectedNil:        true,
			ExpectedFinalizers: []string{chantico.DataCenterResourceGraphFinalizer},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			result := sm.InitializeFinalizer(t.Context(), tc.Case)
			if tc.ExpectedNil {
				if result != nil {
					t.Errorf("InitializeFinalizer(%#v) = %#v, want nil\n", tc, result)
				}
			} else if result == nil || result.PatchType != tc.ExpectedPatchType {
				t.Errorf("InitializeFinalizer(%#v) = %#v -> %#v, want %#v -> %#v\n", tc, result, tc.Case.ObjectMeta.Finalizers, tc.ExpectedPatchType, tc.ExpectedFinalizers)
			}
			if !equalStringSlices(tc.ExpectedFinalizers, tc.Case.ObjectMeta.Finalizers) {
				t.Errorf("InitializeFinalizer(%#v) finalizers = %#v, want %#v\n", tc, tc.Case.ObjectMeta.Finalizers, tc.ExpectedFinalizers)
			}
		})
	}
}

func TestUpdateFinalizer(t *testing.T) {
	testCases := map[string]struct {
		Case               *chantico.DataCenterResource
		ExpectedPatchType  ph.PatchType
		ExpectedFinalizers []string
	}{
		"removes DataCenterResourceGraphFinalizer on deletion": {
			Case: &chantico.DataCenterResource{
				ObjectMeta: metav1.ObjectMeta{
					DeletionTimestamp: &metav1.Time{Time: time.Now()},
					Finalizers:        []string{"test", chantico.DataCenterResourceGraphFinalizer},
				},
			},
			ExpectedPatchType:  ph.PatchResource,
			ExpectedFinalizers: []string{"test"},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			result := sm.RemoveFinalizer(t.Context(), tc.Case)
			if result.PatchType != tc.ExpectedPatchType || !equalStringSlices(tc.ExpectedFinalizers, tc.Case.ObjectMeta.Finalizers) {
				t.Errorf("RemoveFinalizer(%#v) = %#v -> %#v, want %#v -> %#v\n", tc.Case, result, tc.Case.ObjectMeta.Finalizers, tc.ExpectedPatchType, tc.ExpectedFinalizers)
			}
		})
	}
}
