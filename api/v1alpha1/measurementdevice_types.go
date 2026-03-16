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

// Courtesy of the snmp_exporter repository: https://github.com/prometheus/snmp_exporter/blob/main/config/config.go
type Auth struct {
	Community     string `yaml:"community,omitempty" json:"community,omitempty"`
	SecurityLevel string `yaml:"security_level,omitempty" json:"security_level,omitempty"`
	Username      string `yaml:"username,omitempty" json:"username,omitempty"`
	Password      string `yaml:"password,omitempty" json:"password,omitempty"`
	AuthProtocol  string `yaml:"auth_protocol,omitempty" json:"auth_protocol,omitempty"`
	PrivProtocol  string `yaml:"priv_protocol,omitempty" json:"priv_protocol,omitempty"`
	PrivPassword  string `yaml:"priv_password,omitempty" json:"priv_password,omitempty"`
	ContextName   string `yaml:"context_name,omitempty" json:"context_name,omitempty"`
	Version       int    `yaml:"version,omitempty" json:"version,omitempty"`
}

// MeasurementDeviceSpec defines the desired state of MeasurementDevice
type MeasurementDeviceSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of MeasurementDevice. Edit measurementdevice_types.go to remove/update
	Walks []string `yaml:"walks" json:"walks"`
	Auth  Auth     `yaml:"auth" json:"auth"`
}

// MeasurementDeviceStatus defines the observed state of MeasurementDevice
type MeasurementDeviceStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	Conditions       []metav1.Condition `json:"conditions,omitempty"`
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
	MeasurementDeviceFinalizer = "measurementdevice.finalizer.chantico.ci.tno.nl/snmp-update"
)

const (
	RequeueDelay   = 5 * time.Second
	ReloadInterval = 5 * time.Second
	ReloadTimeout  = 3 * time.Minute
	SNMPJobTimeout = 3 * time.Minute
)
