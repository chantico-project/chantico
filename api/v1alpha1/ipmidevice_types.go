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
	User             string              `yaml:"user,omitempty" json:"user,omitempty"`
	Password         string              `yaml:"pass,omitempty" json:"pass,omitempty"`
	Privilege        string              `yaml:"privilege" json:"privilege"`
	Driver           string              `yaml:"driver" json:"driver"`
	Timeout          uint32              `yaml:"timeout" json:"timeout"`
	Collectors       []string            `yaml:"collectors,omitempty" json:"collectors,omitempty"`
	ExcludeSensorIDs []int64             `yaml:"exclude_sensor_ids,omitempty" json:"exclude_sensor_ids,omitempty"`
	WorkaroundFlags  []string            `yaml:"workaround_flags,omitempty" json:"workaround_flags,omitempty"`
	CollectorCmd     map[string]string   `yaml:"collector_cmd,omitempty" json:"collector_cmd,omitempty"`
	CollectorArgs    map[string][]string `yaml:"default_args,omitempty" json:"default_args,omitempty"`
	CustomArgs       map[string][]string `yaml:"custom_args,omitempty" json:"custom_args,omitempty"`

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
	Auth      IPMIConfig `yaml:"auth" json:"auth"`
	SecretRef string     `yaml:"secretRef,omitempty" json:"secretRef,omitempty"`
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

const (
	IPMIUpdateFinalizer = "ipmidevice.finalizer.chantico.ci.tno.nl/ipmi-update"
)

func (r *IPMIDevice) GetState() string            { return r.Status.State }
func (r *IPMIDevice) SetState(s string)           { r.Status.State = s }
func (r *IPMIDevice) GetUpdateGeneration() int64  { return r.Status.UpdateGeneration }
func (r *IPMIDevice) SetUpdateGeneration(g int64) { r.Status.UpdateGeneration = g }
func (r *IPMIDevice) GetFinalizerName() string    { return IPMIUpdateFinalizer }
func (r *IPMIDevice) GetErrorMessage() string     { return r.Status.ErrorMessage }
func (r *IPMIDevice) SetErrorMessage(msg string)  { r.Status.ErrorMessage = msg }
