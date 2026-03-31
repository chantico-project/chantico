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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Courtesy of the ipmi_exporter repository: https://github.com/prometheus-community/ipmi_exporter/blob/master/config.go
type IPMIConfig struct {
	User             string              `yaml:"user" json:"user"`
	Password         string              `yaml:"pass" json:"pass"`
	Privilege        string              `yaml:"privilege" json:"privilege"`
	Driver           string              `yaml:"driver" json:"driver"`
	Timeout          uint32              `yaml:"timeout" json:"timeout"`
	Collectors       []string            `yaml:"collectors" json:"collectors"`
	ExcludeSensorIDs []int64             `yaml:"exclude_sensor_ids" json:"exclude_sensor_ids"`
	WorkaroundFlags  []string            `yaml:"workaround_flags" json:"workaround_flags"`
	CollectorCmd     map[string]string   `yaml:"collector_cmd" json:"collector_cmd"`
	CollectorArgs    map[string][]string `yaml:"default_args" json:"default_args"`
	CustomArgs       map[string][]string `yaml:"custom_args" json:"custom_args"`

	SELEvents []*IpmiSELEvent `yaml:"sel_events,omitempty" json:"sel_events,omitempty"`
}

type IpmiSELEvent struct {
	Name  string `yaml:"name" json:"name"`
	Regex string `yaml:"regex" json:"regex"`
}

// IPMIDeviceSpec defines the desired state of IPMIDevice
type IPMIDeviceSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// IPMI configuration for the exporter
	Auth IPMIConfig `yaml:"auth" json:"auth"`
}

// IPMIDeviceStatus defines the observed state of IPMIDevice
type IPMIDeviceStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	State            string `json:"state,omitempty"`
	UpdateTime       string `json:"updateTime,omitempty"`
	UpdateGeneration int64  `json:"updateGeneration,omitempty"`
	ErrorMessage     string `json:"errorMessage,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// IPMIDevice is the Schema for the ipmidevices API
type IPMIDevice struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IPMIDeviceSpec   `json:"spec,omitempty"`
	Status IPMIDeviceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// IPMIDeviceList contains a list of IPMIDevice
type IPMIDeviceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IPMIDevice `json:"items"`
}

func init() {
	SchemeBuilder.Register(&IPMIDevice{}, &IPMIDeviceList{})
}
