/*
Copyright 2022.

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

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// AutohealConfigSpec defines the desired state of AutohealConfig
type AutohealConfigSpec struct {
	SyncPeriod string `json:"syncPeriod,omitempty"`
	SecretName string `json:"secretName,omitempty"`
}

// AutohealConfigStatus defines the observed state of AutohealConfig
type AutohealConfigStatus struct {
	LastSyncPeriod int64 `json:"lastSyncPeriod,omitempty"`
}
type ConfigRepositoryInfo struct {
	Tag           string
	AccessToken   string
	RepositoryURL string
}
type AutohealConfigTree struct {
	Tree []AutohealConfigTreeResponse
}
type AutohealConfigTreeResponse struct {
	Filename string `json:"path"`
}

type AutohealConfigScenario struct {
	Name        string `yaml:"name"`
	Query       string `yaml:"query"`
	Label       string `yaml:"label"`
	Script      string `yaml:"script"`
	GracePeriod string `yaml:"gracePeriod"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// AutohealConfig is the Schema for the autohealconfigs API
type AutohealConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AutohealConfigSpec   `json:"spec,omitempty"`
	Status AutohealConfigStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// AutohealConfigList contains a list of AutohealConfig
type AutohealConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AutohealConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AutohealConfig{}, &AutohealConfigList{})
}
