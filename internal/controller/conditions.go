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
	"time"

	chantico "chantico/api/v1alpha1"
	"chantico/internal/steps"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func reconciled(obj chantico.ConditionsObject, condType chantico.ConditionType, msg string) steps.StepResult {
	obj.UpdateStatusCondition(condType, metav1.ConditionTrue, chantico.ReasonReconciled, msg)
	return steps.Continue()
}

func progressing(obj chantico.ConditionsObject, condType chantico.ConditionType, reason chantico.ConditionReason, msg string) steps.StepResult {
	obj.UpdateStatusCondition(condType, metav1.ConditionUnknown, reason, msg)
	return steps.Continue()
}

func waiting(obj chantico.ConditionsObject, condType chantico.ConditionType, reason chantico.ConditionReason, msg string) steps.StepResult {
	obj.UpdateStatusCondition(condType, metav1.ConditionUnknown, reason, msg)
	return steps.Stop()
}

func halted(obj chantico.ConditionsObject, condType chantico.ConditionType, reason chantico.ConditionReason, msg string) steps.StepResult {
	obj.UpdateStatusCondition(condType, metav1.ConditionFalse, reason, msg)
	return steps.Stop()
}

func requeued(obj chantico.ConditionsObject, condType chantico.ConditionType, reason chantico.ConditionReason, msg string, after time.Duration) steps.StepResult {
	obj.UpdateStatusCondition(condType, metav1.ConditionFalse, reason, msg)
	return steps.Requeue(after)
}

func failed(obj chantico.ConditionsObject, condType chantico.ConditionType, reason chantico.ConditionReason, msg string, err error) steps.StepResult {
	obj.UpdateStatusCondition(condType, metav1.ConditionFalse, reason, msg+": "+err.Error())
	return steps.Error(err)
}
