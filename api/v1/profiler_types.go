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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ProfilerSpec defines the desired state of Profiler
type ProfilerSpec struct {

	// CPU threshold in percentage (0-100) that triggers profiling
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	CPUThreshold *int32 `json:"cpuThreshold,omitempty"`

	// Memory threshold in percentage (0-100) that triggers profiling
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	MemoryThreshold *int32 `json:"memoryThreshold,omitempty"`

	// Monitoring period in seconds to check resource usage
	// +optional
	// +kubebuilder:default=15
	MonitoringPeriod int32 `json:"monitoringPeriod,omitempty"`

	// S3 bucket to upload profiles to
	S3Bucket string `json:"s3Bucket"`

	// S3 region
	// +optional
	// +kubebuilder:default=us-east-1
	S3Region string `json:"s3Region,omitempty"`

	// S3 path prefix for storing profiles
	// +optional
	S3PathPrefix string `json:"s3PathPrefix,omitempty"`

	// AWS credentials secret name
	// +optional
	AWSCredentialsSecret string `json:"awsCredentialsSecret,omitempty"`

	// ScrapTarget is the URL to scrape profiles from
	// +optional
	ScrapTarget *ScrapTarget `json:"scrapURL,omitempty"`
}

// ScrapTarget Details
type ScrapTarget struct {

	// ScrapURL Scrap Endpoint
	// +optional
	ScrapeURL string `json:"scrapeURL,omitempty"`

	// Auth Scrap Endpoint authentication detail
	// +optional
	Auth *Auth `json:"auth,omitempty,omitempty"`
}

// AuthType defines the type of authentication
// +kubebuilder:validation:Enum=Basic;None
type AuthType string

const (
	// AuthTypeBasic represents basic authentication
	AuthTypeBasic AuthType = "Basic"
)

// Auth contains authentication configuration
type Auth struct {
	// Type of authentication to use
	// +kubebuilder:default=Basic
	Type AuthType `json:"type"`

	// BasicAuth contains credentials for basic authentication
	// +optional
	BasicAuth *BasicAuth `json:"basicAuth,omitempty"`
}

// BasicAuth contains credentials for basic authentication
type BasicAuth struct {
	// Username for basic authentication
	// +optional
	Username string `json:"username,omitempty"`

	// Password for basic authentication
	// +optional
	Password string `json:"password,omitempty"`

	// SecretRef references a secret that contains the credentials
	// +optional
	SecretRef *SecretRef `json:"secretRef,omitempty"`
}

// SecretRef contains a reference to a secret
type SecretRef struct {
	// Name of the secret
	Name string `json:"name"`

	// Namespace of the secret
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// UsernameKey is the key in the secret that contains the username
	// +optional
	// +kubebuilder:default=username
	UsernameKey string `json:"usernameKey,omitempty"`

	// PasswordKey is the key in the secret that contains the password
	// +optional
	// +kubebuilder:default=password
	PasswordKey string `json:"passwordKey,omitempty"`
}

// ProfilerStatus defines the observed state of Profiler.
type ProfilerStatus struct {
	// List of profiles that have been collected
	// +optional
	Profiles []ProfileRecord `json:"profiles,omitempty"`
}

// ProfileRecord contains information about a collected profile
type ProfileRecord struct {
	// Timestamp when the profile was collected
	Timestamp metav1.Time `json:"timestamp"`

	// Type of profile (cpu, memory, etc.)
	Type string `json:"type"`

	// S3 URL where the profile is stored
	S3URL string `json:"s3URL"`

	// Reason why the profile was triggered
	Reason string `json:"reason"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Profiler is the Schema for the profilers API
type Profiler struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of Profiler
	// +required
	Spec ProfilerSpec `json:"spec"`

	// status defines the observed state of Profiler
	// +optional
	Status ProfilerStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// ProfilerList contains a list of Profiler
type ProfilerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Profiler `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Profiler{}, &ProfilerList{})
}
