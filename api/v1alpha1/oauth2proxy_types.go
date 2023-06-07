/*
Copyright 2023.

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

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// OAuth2ProxySpec defines the desired state of OAuth2Proxy
type OAuth2ProxySpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Size is the number of replicas
	// +kubebuilder:validation:Required
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	Size int32 `json:"size"`

	// Config is the configuration file to pass to oAuth2Proxy
	// +kubebuilder:validation:Required
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	Config string `json:"config,omitempty"`
}

// OAuth2ProxyStatus defines the observed state of OAuth2Proxy
type OAuth2ProxyStatus struct {
	// Represents the observations of a OAuth2Proxy's current state.
	// OAuth2Proxy.status.conditions.type are: "Available", "Progressing", and "Degraded"
	// OAuth2Proxy.status.conditions.status are one of True, False, Unknown.
	// OAuth2Proxy.status.conditions.reason the value should be a CamelCase string and producers of specific
	// condition types may define expected values and meanings for this field, and whether the values
	// are considered a guaranteed API.
	// OAuth2Proxy.status.conditions.Message is a human readable message indicating details about the transition.
	// For further information see: https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// OAuth2Proxy is the Schema for the oauth2proxies API
type OAuth2Proxy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OAuth2ProxySpec   `json:"spec,omitempty"`
	Status OAuth2ProxyStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// OAuth2ProxyList contains a list of OAuth2Proxy
type OAuth2ProxyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OAuth2Proxy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OAuth2Proxy{}, &OAuth2ProxyList{})
}
