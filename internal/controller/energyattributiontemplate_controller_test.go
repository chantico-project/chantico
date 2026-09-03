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
	"testing"

	chantico "chantico/api/v1alpha1"
	"chantico/internal/steps"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestEnergyAttributionTemplateReconcilerReconcileReady(t *testing.T) {
	testCases := map[string]struct {
		template       string
		expectedAction steps.Action
		expectedStatus metav1.ConditionStatus
		expectedReason chantico.ConditionReason
	}{
		"accepts a valid template": {
			template:       "{{ .variable }} * 0.5",
			expectedAction: steps.ActionContinue,
			expectedStatus: metav1.ConditionTrue,
			expectedReason: chantico.ReasonReconciled,
		},
		"rejects an invalid template": {
			template:       "{{ .variable ",
			expectedAction: steps.ActionError,
			expectedStatus: metav1.ConditionFalse,
			expectedReason: chantico.ReasonInvalidSpec,
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			resource := &chantico.EnergyAttributionTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "template"},
				Spec:       chantico.EnergyAttributionTemplateSpec{Template: testCase.template},
			}

			result := (&EnergyAttributionTemplateReconciler{}).reconcileReady(t.Context(), resource)
			if result.Action != testCase.expectedAction {
				t.Fatalf("reconcileReady() action = %v, want %v", result.Action, testCase.expectedAction)
			}

			condition := meta.FindStatusCondition(resource.Status.Conditions, string(chantico.ConditionReady))
			if condition == nil {
				t.Fatal("reconcileReady() did not set Ready condition")
			}
			if condition.Status != testCase.expectedStatus {
				t.Errorf("Ready condition status = %q, want %q", condition.Status, testCase.expectedStatus)
			}
			if condition.Reason != string(testCase.expectedReason) {
				t.Errorf("Ready condition reason = %q, want %q", condition.Reason, testCase.expectedReason)
			}
		})
	}
}
