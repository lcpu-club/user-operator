package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// UserCreationConfigSpec defines the desired state of UserCreationConfig.
type UserCreationConfigSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Enabled         bool     `json:"enabled"`
	NamespacePrefix string   `json:"namespacePrefix,omitempty"`
	Resources       []string `json:"resources,omitempty"`
}

// UserCreationConfigStatus defines the observed state of UserCreationConfig.
type UserCreationConfigStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster

// UserCreationConfig is the Schema for the usercreationconfigs API.
type UserCreationConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   UserCreationConfigSpec   `json:"spec,omitempty"`
	Status UserCreationConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// UserCreationConfigList contains a list of UserCreationConfig.
type UserCreationConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []UserCreationConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&UserCreationConfig{}, &UserCreationConfigList{})
}
