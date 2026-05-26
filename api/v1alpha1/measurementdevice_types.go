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
	"chantico/internal/snmp"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MeasurementDeviceSpec defines the desired state of MeasurementDevice
type MeasurementDeviceSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Walks []string           `yaml:"walks" json:"walks"`
	Auth  snmp.GeneratorAuth `yaml:"auth" json:"auth"`
}

// MeasurementDeviceStatus defines the observed state of MeasurementDevice
type MeasurementDeviceStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	ConfigHash         string             `json:"configHash,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=md;msd
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`

// MeasurementDevice is the Schema for the measurementdevices API
type MeasurementDevice struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MeasurementDeviceSpec   `json:"spec,omitempty"`
	Status MeasurementDeviceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// MeasurementDeviceList contains a list of MeasurementDevice
type MeasurementDeviceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MeasurementDevice `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MeasurementDevice{}, &MeasurementDeviceList{})
}

const (
	SNMPUpdateFinalizer = "measurementdevice.finalizer.chantico-project.github.io/snmp-update"
)

const (
	RequeueDelay   = 5 * time.Second
	ReloadInterval = 5 * time.Second
	ReloadTimeout  = 3 * time.Minute
	SNMPJobTimeout = 3 * time.Minute
)

type ConditionType string

const (
	ConditionJob            ConditionType = "Job"
	ConditionConfig         ConditionType = "Config"
	ConditionGeneratorFile  ConditionType = "GeneratorFile"
	ConditionExporterReload ConditionType = "ExporterReload"
)

type ConditionReason string

const (
	ReasonPending     ConditionReason = "Pending"
	ReasonFailed      ConditionReason = "Failed"
	ReasonSucceeded   ConditionReason = "Succeeded"
	ReasonFileWritten ConditionReason = "FileWritten"
	ReasonSynced      ConditionReason = "Synced"
)

func (m *MeasurementDevice) GetConditions() *[]metav1.Condition { return &m.Status.Conditions }

func (m *MeasurementDevice) UpdateStatusCondition(t ConditionType, s metav1.ConditionStatus, reason ConditionReason, msg string) {
	meta.SetStatusCondition(m.GetConditions(), metav1.Condition{
		Type: string(t), Status: s, Reason: string(reason), Message: msg,
		ObservedGeneration: m.GetGeneration(),
	})
}

func (m *MeasurementDevice) UpdateStatusJobCondition(condition *metav1.Condition) {
	meta.SetStatusCondition(m.GetConditions(), metav1.Condition{
		Type: string(ConditionJob), Status: condition.Status, Reason: condition.Reason, Message: condition.Message,
		ObservedGeneration: m.GetGeneration(),
	})
}
