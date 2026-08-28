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

// ParentRef references a parent DataCenterResource and optionally carries
// the energy coefficient for the edge from that parent to this node.
// The coefficient represents what fraction of the parent's energy is
// attributable to this child.
type ParentRef struct {
	// Name is the name of the parent DataCenterResource.
	Name string `json:"name"`

	// Coefficient is the energy coefficient for the edge from this parent
	// to the current node. It is a PromQL expression (often a literal
	// number) that will be written as a Prometheus recording rule.
	// +optional
	Coefficient string `json:"coefficient,omitempty"`
}

// DataCenterResourceSpec defines the desired state of DataCenterResource
type DataCenterResourceSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Type string `json:"type"`

	// +optional
	PhysicalMeasurements []string `json:"physicalMeasurements,omitempty"`

	// Parents lists the parent DataCenterResources that feed energy into
	// this node. Each entry carries an optional coefficient representing
	// the proportional share of the parent's energy attributable to this
	// child. The coefficient is written as a Prometheus recording rule so
	// that the energy formula can reference it.
	// +optional
	Parents []ParentRef `json:"parents,omitempty"`

	// EnergyMetric is the Prometheus metric name for the raw energy timeseries
	// of this resource. Only set for root nodes (e.g. PDUs) whose energy
	// timeseries is produced directly by hardware / an exporter.
	// +optional
	EnergyMetric string `json:"energyMetric,omitempty"`

	// ServiceId is the identifier of the service that this resource belongs to.
	// Define on a resource to make it part of a service, or make a separate
	// data center resource with service type with parent resources.
	// +optional
	ServiceId string `json:"serviceId"`
}

// DataCenterResourceStatus defines the observed state of DataCenterResource.
type DataCenterResourceStatus struct {
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
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.conditions[-1].status`
// +kubebuilder:printcolumn:name="Reason",type=string,JSONPath=`.status.conditions[-1].reason`
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.status.conditions[-1].type`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// DataCenterResource is the Schema for the datacenterresources API
type DataCenterResource struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of DataCenterResource
	// +required
	Spec DataCenterResourceSpec `json:"spec"`

	// status defines the observed state of DataCenterResource
	// +optional
	Status DataCenterResourceStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// DataCenterResourceList contains a list of DataCenterResource
type DataCenterResourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []DataCenterResource `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DataCenterResource{}, &DataCenterResourceList{})
}

const (
	DataCenterResourceGraphFinalizer = "datacenterresource.finalizer.chantico-project.github.io/graph"
)

func (m *DataCenterResource) GetConditions() *[]metav1.Condition { return &m.Status.Conditions }

func (m *DataCenterResource) UpdateStatusCondition(t ConditionType, s metav1.ConditionStatus, reason ConditionReason, msg string) {
	meta.SetStatusCondition(m.GetConditions(), metav1.Condition{
		Type: string(t), Status: s, Reason: string(reason), Message: msg,
		ObservedGeneration: m.GetGeneration(),
	})
}

// ParentNames returns a flat list of parent resource names, for use in
// validation, indexing, and anywhere the full ParentRef is not needed.
func (s *DataCenterResourceSpec) ParentNames() []string {
	if len(s.Parents) == 0 {
		return nil
	}
	names := make([]string, len(s.Parents))
	for i, p := range s.Parents {
		names[i] = p.Name
	}
	return names
}
