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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// PhysicalMeasurementSpec defines the desired state of PhysicalMeasurement
type PhysicalMeasurementSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Ip          string   `json:"ip"`
	ServiceId   string   `json:"serviceId"`
	SNMPDevice  string   `json:"snmpDevice"`
	ResourceIds []string `json:"resourceIds,omitempty"`
}

// PhysicalMeasurementStatus defines the observed state of PhysicalMeasurement
type PhysicalMeasurementStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	State            string `json:"state,omitempty"`
	JobName          string `json:"jobName,omitempty"`
	UpdateTime       string `json:"updateTime,omitempty"`
	UpdateGeneration int64  `json:"updateGeneration,omitempty"`
	ErrorMessage     string `json:"errorMessage,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

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
	PhysicalMeasurementFinalizer = "physicalmeasurement.chantico.ci.tno.nl/finalizer"
)

func (r *PhysicalMeasurement) GetState() string            { return r.Status.State }
func (r *PhysicalMeasurement) SetState(s string)           { r.Status.State = s }
func (r *PhysicalMeasurement) GetUpdateGeneration() int64  { return r.Status.UpdateGeneration }
func (r *PhysicalMeasurement) SetUpdateGeneration(g int64) { r.Status.UpdateGeneration = g }
func (r *PhysicalMeasurement) GetFinalizerName() string    { return PhysicalMeasurementFinalizer }
func (r *PhysicalMeasurement) GetErrorMessage() string     { return r.Status.ErrorMessage }
func (r *PhysicalMeasurement) SetErrorMessage(msg string)  { r.Status.ErrorMessage = msg }

func init() {
	SchemeBuilder.Register(&PhysicalMeasurement{}, &PhysicalMeasurementList{})
}

const (
	EndpointRequeueDelay = 30 * time.Second
)
