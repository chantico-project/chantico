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
	chantico "chantico/api/v1alpha1"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type healthConditionResource interface {
	GetConditions() []metav1.Condition
	HealthConditions() []chantico.ConditionType
	UpdateStatusCondition(chantico.ConditionType, metav1.ConditionStatus, chantico.ConditionReason, string)
}

// initHealthConditions sets missing health conditions to True. Existing ones are left to the alert
// webhook, so that reconciling a resource does not hide an alert that still fires.
func initHealthConditions(resource healthConditionResource) {
	for _, condition := range resource.HealthConditions() {
		if meta.FindStatusCondition(resource.GetConditions(), string(condition)) == nil {
			resource.UpdateStatusCondition(condition, metav1.ConditionTrue, chantico.ReasonReconciled, "No health alert has fired")
		}
	}
}
