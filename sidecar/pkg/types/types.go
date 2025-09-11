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

package types

// ScrapTarget Details
type ScrapTarget struct {
	// ScrapURL Scrap Endpoint
	ScrapeURL string `json:"scrapeURL,omitempty"`

	// Auth Scrap Endpoint authentication detail
	Auth *Auth `json:"auth,omitempty,omitempty"`
}

// AuthType defines the type of authentication
type AuthType string

const (
	// AuthTypeBasic represents basic authentication
	AuthTypeBasic AuthType = "Basic"
)

// Auth contains authentication configuration
type Auth struct {
	// Type of authentication to use
	Type AuthType `json:"type"`

	// BasicAuth contains credentials for basic authentication
	BasicAuth *BasicAuth `json:"basicAuth,omitempty"`
}

// BasicAuth contains credentials for basic authentication
type BasicAuth struct {
	// Username for basic authentication
	Username string `json:"username,omitempty"`

	// Password for basic authentication
	Password string `json:"password,omitempty"`

	// SecretRef references a secret that contains the credentials
	SecretRef *SecretRef `json:"secretRef,omitempty"`
}

// SecretRef contains a reference to a secret
type SecretRef struct {
	// Name of the secret
	Name string `json:"name"`

	// Namespace of the secret
	Namespace string `json:"namespace,omitempty"`

	// UsernameKey is the key in the secret that contains the username
	UsernameKey string `json:"usernameKey,omitempty"`

	// PasswordKey is the key in the secret that contains the password
	PasswordKey string `json:"passwordKey,omitempty"`
}
