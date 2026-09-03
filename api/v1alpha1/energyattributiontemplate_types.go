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

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// EnergyAttributionTemplateSpec defines the desired state of EnergyAttributionTemplate
type EnergyAttributionTemplateSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Template   string   `json:"template"`
	Parameters []string `json:"parameters"`
}

// EnergyAttributionTemplateStatus defines the observed state of EnergyAttributionTemplate
type EnergyAttributionTemplateStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	UpdateTime       string             `json:"updateTime,omitempty"`
	UpdateGeneration int64              `json:"updateGeneration,omitempty"`
	ErrorMessage     string             `json:"errorMessage,omitempty"`
	ErrorType        string             `json:"errorType,omitempty"`
	InvolvedResource string             `json:"involvedResource,omitempty"`
	Conditions       []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// EnergyAttributionTemplate is the Schema for the energyattributiontemplate API
type EnergyAttributionTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EnergyAttributionTemplateSpec   `json:"spec,omitempty"`
	Status EnergyAttributionTemplateStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EnergyAttributionTemplateList contains a list of EnergyAttributionTemplate
type EnergyAttributionTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EnergyAttributionTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EnergyAttributionTemplate{}, &EnergyAttributionTemplateList{})
}

const (
	EnergyAttributionTemplateFinalizer = "energyattributiontemplate.chantico-project.github.io/finalizer"
)

func (m *EnergyAttributionTemplate) GetConditions() *[]metav1.Condition { return &m.Status.Conditions }

func (m *EnergyAttributionTemplate) UpdateStatusCondition(t ConditionType, s metav1.ConditionStatus, reason ConditionReason, msg string) {
	meta.SetStatusCondition(m.GetConditions(), metav1.Condition{
		Type: string(t), Status: s, Reason: string(reason), Message: msg,
		ObservedGeneration: m.GetGeneration(),
	})
}
