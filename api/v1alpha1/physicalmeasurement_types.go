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
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// PhysicalMeasurementSpec defines the desired state of PhysicalMeasurement
type PhysicalMeasurementSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Ip                string   `json:"ip"`
	MeasurementDevice string   `json:"measurementDevice"`
	ResourceIds       []string `json:"resourceIds,omitempty"`
}

// PhysicalMeasurementStatus defines the observed state of PhysicalMeasurement
type PhysicalMeasurementStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	JobName          string             `json:"jobName,omitempty"`
	UpdateTime       string             `json:"updateTime,omitempty"`
	UpdateGeneration int64              `json:"updateGeneration,omitempty"`
	Conditions       []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.conditions[-1].status`
// +kubebuilder:printcolumn:name="Reason",type=string,JSONPath=`.status.conditions[-1].reason`
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.status.conditions[-1].type`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// PhysicalMeasurement is the Schema for the physicalmeasurements API
type PhysicalMeasurement struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PhysicalMeasurementSpec   `json:"spec,omitempty"`
	Status PhysicalMeasurementStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PhysicalMeasurementList contains a list of PhysicalMeasurement
type PhysicalMeasurementList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PhysicalMeasurement `json:"items"`
}

const (
	PhysicalMeasurementFinalizer = "physicalmeasurement.chantico-project.github.io/finalizer"
)

func (m *PhysicalMeasurement) GetConditions() *[]metav1.Condition { return &m.Status.Conditions }

func (m *PhysicalMeasurement) UpdateStatusCondition(t ConditionType, s metav1.ConditionStatus, reason ConditionReason, msg string) {
	meta.SetStatusCondition(m.GetConditions(), metav1.Condition{
		Type: string(t), Status: s, Reason: string(reason), Message: msg,
		ObservedGeneration: m.GetGeneration(),
	})
}

func init() {
	SchemeBuilder.Register(&PhysicalMeasurement{}, &PhysicalMeasurementList{})
}

const (
	EndpointRequeueDelay = 30 * time.Second
)
