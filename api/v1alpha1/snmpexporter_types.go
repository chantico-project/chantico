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

type SNMPExporterSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	DeviceSelector *metav1.LabelSelector `json:"deviceSelector,omitempty"`

	// +optional
	Replicas *int32 `json:"replicas,omitempty"`
}

type SNMPExporterStatus struct {
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	ConfigHash         string             `json:"configHash,omitempty"`
	DeviceCount        int32              `json:"deviceCount,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Devices",type=integer,JSONPath=`.status.deviceCount`
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
type SNMPExporter struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              SNMPExporterSpec   `json:"spec,omitempty"`
	Status            SNMPExporterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type SNMPExporterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SNMPExporter `json:"items"`
}

const (
	SNMPExporterFinalizer = "snmpexporter.finalizer.chantico.ci.tno.nl"
)

const (
	ConditionMergedConfig ConditionType = "MergedConfig"
	ConditionDeployment   ConditionType = "Deployment"

	ReasonMerged          ConditionReason = "Merged"
	ReasonMergeFailed     ConditionReason = "MergeFailed"
	ReasonDeploymentReady ConditionReason = "DeploymentReady"
	ReasonRollingOut      ConditionReason = "RollingOut"
)

func (e *SNMPExporter) GetConditions() *[]metav1.Condition { return &e.Status.Conditions }

func (e *SNMPExporter) UpdateStatusCondition(t ConditionType, s metav1.ConditionStatus, reason ConditionReason, msg string) {
	meta.SetStatusCondition(e.GetConditions(), metav1.Condition{
		Type: string(t), Status: s, Reason: string(reason), Message: msg,
		ObservedGeneration: e.GetGeneration(),
	})
}

func init() { SchemeBuilder.Register(&SNMPExporter{}, &SNMPExporterList{}) }
