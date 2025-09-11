package v1

import (
	"fmt"
	"github.com/go-logr/logr"
	observabilityv1 "github.com/maulindesai/pprof-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
)

// HasSidecar checks if a container with the given name exists in the list of containers
func HasSidecar(containers []corev1.Container, name string) bool {
	for _, container := range containers {
		if container.Name == name {
			return true
		}
	}
	return false
}

// getConfiguredCPUThreshold returns the CPU threshold configuration as an environment variable
// It reads the CPU threshold from the CRD
func getConfiguredCPUThreshold(profiler *observabilityv1.Profiler, logger logr.Logger) []corev1.EnvVar {
	logger.V(1).Info("Configuring CPU threshold")

	var envVars []corev1.EnvVar

	if profiler.Spec.CPUThreshold != nil {
		logger.Info("Using CPU threshold from CRD",
			"value", *profiler.Spec.CPUThreshold)
		envVars = append(envVars, corev1.EnvVar{
			Name:  "CPU_THRESHOLD",
			Value: fmt.Sprintf("%d", *profiler.Spec.CPUThreshold),
		})
	} else {
		logger.V(1).Info("No CPU threshold configured in CRD")
	}

	return envVars
}

// getConfiguredMemoryThreshold returns the memory threshold configuration as an environment variable
// It reads the memory threshold from the CRD
func getConfiguredMemoryThreshold(profiler *observabilityv1.Profiler, logger logr.Logger) []corev1.EnvVar {
	logger.V(1).Info("Configuring memory threshold")

	var envVars []corev1.EnvVar

	if profiler.Spec.MemoryThreshold != nil {
		logger.Info("Using memory threshold from CRD",
			"value", *profiler.Spec.MemoryThreshold)
		envVars = append(envVars, corev1.EnvVar{
			Name:  "MEMORY_THRESHOLD",
			Value: fmt.Sprintf("%d", *profiler.Spec.MemoryThreshold),
		})
	} else {
		logger.V(1).Info("No memory threshold configured in CRD")
	}

	return envVars
}

// getAWSConfiguration returns the AWS configuration as environment variables
// This includes S3 bucket, region, path prefix, and AWS credentials if specified
func getAWSConfiguration(profiler *observabilityv1.Profiler, logger logr.Logger) []corev1.EnvVar {
	logger.V(1).Info("Configuring S3 storage",
		"bucket", profiler.Spec.S3Bucket,
		"region", profiler.Spec.S3Region,
		"pathPrefix", profiler.Spec.S3PathPrefix)

	envVars := []corev1.EnvVar{
		{
			Name:  "S3_BUCKET",
			Value: profiler.Spec.S3Bucket,
		},
		{
			Name:  "S3_REGION",
			Value: profiler.Spec.S3Region,
		},
		{
			Name:  "S3_PATH_PREFIX",
			Value: profiler.Spec.S3PathPrefix,
		},
	}

	// Add AWS credentials from secret if specified
	if profiler.Spec.AWSCredentialsSecret != "" {
		logger.V(1).Info("Configuring AWS credentials from secret",
			"secret", profiler.Spec.AWSCredentialsSecret)

		envVars = append(envVars, []corev1.EnvVar{
			{
				Name: "AWS_ACCESS_KEY_ID",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: profiler.Spec.AWSCredentialsSecret,
						},
						Key: "AWS_ACCESS_KEY_ID",
					},
				},
			},
			{
				Name: "AWS_SECRET_ACCESS_KEY",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: profiler.Spec.AWSCredentialsSecret,
						},
						Key: "AWS_SECRET_ACCESS_KEY",
					},
				},
			},
		}...)
	} else {
		logger.V(1).Info("No AWS credentials secret specified, assuming instance role or environment credentials")
	}

	return envVars
}

// getScrapeTargetConfiguration returns environment variables for scraping target configuration
// It reads the scrape URL and authentication settings from pod annotations first,
// then falls back to the CRD values if annotations are not present
// Parameters:
//   - profiler: The Profiler CR containing scrape target configuration
//   - pod: The pod being configured, which may have annotation overrides
//   - logger: Logger for recording configuration decisions
//
// Returns:
//   - []corev1.EnvVar: Environment variables for scrape target configuration
func getScrapeTargetConfiguration(profiler *observabilityv1.Profiler, pod *corev1.Pod, logger logr.Logger) []corev1.EnvVar {
	var envVars []corev1.EnvVar
	logger.V(1).Info("Configuring ScrapeTarget")

	// Configure scrape URL
	envVars = append(envVars, getScrapeTargetURL(profiler, pod, logger)...)

	// Configure authentication
	authType := getScrapeTargetAuthType(profiler, pod, logger)
	if authType.Value != "" {
		envVars = append(envVars, authType)

		// Configure basic auth if specified
		if authType.Value == string(observabilityv1.AuthTypeBasic) {
			envVars = append(envVars, getScrapeTargetBasicAuth(profiler, pod, logger)...)
		} else {
			logger.V(1).Info("No authentication configured or not supported", "authType", authType.Value)
		}
	}

	return envVars
}

// getScrapeTargetAuthType determines the authentication type for scraping target
// It first checks for an annotation-based auth type, then falls back to the CRD value
// Parameters:
//   - profiler: The Profiler CR containing auth type configuration
//   - pod: The pod being configured, which may have annotation overrides
//   - logger: Logger for recording configuration decisions
//
// Returns:
//   - corev1.EnvVar: The authentication type environment variable
func getScrapeTargetAuthType(profiler *observabilityv1.Profiler, pod *corev1.Pod, logger logr.Logger) corev1.EnvVar {
	// First check for annotation-based auth type
	authTypeAnnotation, hasAuthTypeAnnotation := pod.Annotations[AnnotationScrapeAuthType]
	if hasAuthTypeAnnotation && authTypeAnnotation != "" {
		logger.Info("Using auth type from annotation",
			"value", authTypeAnnotation,
			"annotation", AnnotationScrapeAuthType)
		return corev1.EnvVar{
			Name:  "AUTH_TYPE",
			Value: authTypeAnnotation,
		}
	}

	// Fall back to CRD-based auth type
	if profiler.Spec.ScrapTarget != nil && profiler.Spec.ScrapTarget.Auth != nil {
		authType := profiler.Spec.ScrapTarget.Auth.Type
		logger.Info("Using auth type from CRD", "value", authType)
		return corev1.EnvVar{
			Name:  "AUTH_TYPE",
			Value: string(authType),
		}
	}

	// No auth type configured
	logger.V(1).Info("No authentication type configured")
	return corev1.EnvVar{
		Name:  "AUTH_TYPE",
		Value: "",
	}
}

// getScrapeTargetURL returns environment variables for the scrape URL configuration
// It first checks for a URL annotation, then falls back to the CRD value
// Parameters:
//   - profiler: The Profiler CR containing scrape URL configuration
//   - pod: The pod being configured, which may have annotation overrides
//   - logger: Logger for recording configuration decisions
//
// Returns:
//   - []corev1.EnvVar: Environment variables for scrape URL configuration
func getScrapeTargetURL(profiler *observabilityv1.Profiler, pod *corev1.Pod, logger logr.Logger) []corev1.EnvVar {
	var envVars []corev1.EnvVar

	// Check for annotation-based URL
	scrapeURLAnnotation, hasScrapeURLAnnotation := pod.Annotations[AnnotationScrapeURL]
	if hasScrapeURLAnnotation && scrapeURLAnnotation != "" {
		logger.Info("Using scrape URL from annotation",
			"value", scrapeURLAnnotation,
			"annotation", AnnotationScrapeURL)
		envVars = append(envVars, corev1.EnvVar{
			Name:  "SCRAPE_URL",
			Value: scrapeURLAnnotation,
		})
		return envVars
	}

	// Fall back to CRD-based URL
	if profiler.Spec.ScrapTarget != nil && profiler.Spec.ScrapTarget.ScrapeURL != "" {
		logger.Info("Using scrape URL from CRD", "value", profiler.Spec.ScrapTarget.ScrapeURL)
		envVars = append(envVars, corev1.EnvVar{
			Name:  "SCRAPE_URL",
			Value: profiler.Spec.ScrapTarget.ScrapeURL,
		})
		return envVars
	}

	logger.V(1).Info("No scrape URL configured")
	return envVars
}

// getScrapeTargetBasicAuth returns environment variables for basic authentication
// It first checks for annotation-based auth, then falls back to the CRD values
// Parameters:
//   - profiler: The Profiler CR containing auth configuration
//   - pod: The pod being configured, which may have annotation overrides
//   - logger: Logger for recording configuration decisions
//
// Returns:
//   - []corev1.EnvVar: Environment variables for basic authentication
func getScrapeTargetBasicAuth(profiler *observabilityv1.Profiler, pod *corev1.Pod, logger logr.Logger) []corev1.EnvVar {
	// Check annotation-based direct auth first
	if username, password, ok := getAnnotationAuth(pod); ok {
		logger.Info("Using basic auth credentials from annotations")
		return buildBasicAuthEnvVars(username, password)
	}

	// Check secret annotation auth
	if secret, ok := getSecretAnnotationAuth(pod); ok {
		logger.Info("Using basic auth credentials from secret specified in annotation",
			"secret", secret.name,
			"usernameKey", secret.usernameKey,
			"passwordKey", secret.passwordKey)
		return buildSecretAuthEnvVars(secret.name, secret.usernameKey, secret.passwordKey)
	}

	// Check CRD-based auth
	if auth := getCRDAuth(profiler); auth != nil {
		if auth.directAuth {
			logger.Info("Using basic auth credentials from CRD")
			return buildBasicAuthEnvVars(auth.username, auth.password)
		}

		if auth.secretRef != nil {
			logger.Info("Using basic auth credentials from secret specified in CRD",
				"secret", auth.secretRef.name,
				"usernameKey", auth.secretRef.usernameKey,
				"passwordKey", auth.secretRef.passwordKey)
			return buildSecretAuthEnvVars(auth.secretRef.name, auth.secretRef.usernameKey, auth.secretRef.passwordKey)
		}

		logger.V(1).Info("CRD has Auth section but no valid credentials configuration")
		return []corev1.EnvVar{}
	}

	logger.V(1).Info("No authentication credentials configured")
	return []corev1.EnvVar{}
}

// secretAuth represents authentication credentials stored in a secret
type secretAuth struct {
	name        string
	usernameKey string
	passwordKey string
}

// getAnnotationAuth extracts username and password from pod annotations
// Returns username, password, and a boolean indicating if both are present
func getAnnotationAuth(pod *corev1.Pod) (username, password string, ok bool) {
	username, hasUsername := pod.Annotations[AnnotationScrapeAuthUsername]
	password, hasPassword := pod.Annotations[AnnotationScrapeAuthPassword]
	return username, password, hasUsername && hasPassword && username != "" && password != ""
}

// getSecretAnnotationAuth extracts secret reference from pod annotations
// Returns a secretAuth struct and a boolean indicating if the secret is specified
func getSecretAnnotationAuth(pod *corev1.Pod) (*secretAuth, bool) {
	secret, hasSecret := pod.Annotations[AnnotationScrapeAuthSecret]
	if !hasSecret || secret == "" {
		return nil, false
	}

	usernameKey := pod.Annotations[AnnotationScrapeAuthUsernameKey]
	if usernameKey == "" {
		usernameKey = "username"
	}

	passwordKey := pod.Annotations[AnnotationScrapeAuthPasswordKey]
	if passwordKey == "" {
		passwordKey = "password"
	}

	return &secretAuth{
		name:        secret,
		usernameKey: usernameKey,
		passwordKey: passwordKey,
	}, true
}

// crdAuth represents authentication configuration from the CRD
type crdAuth struct {
	directAuth bool
	username   string
	password   string
	secretRef  *secretAuth
}

// getCRDAuth extracts authentication configuration from the CRD
// Returns a crdAuth struct or nil if no auth is configured
func getCRDAuth(profiler *observabilityv1.Profiler) *crdAuth {
	if profiler.Spec.ScrapTarget == nil ||
		profiler.Spec.ScrapTarget.Auth == nil ||
		profiler.Spec.ScrapTarget.Auth.BasicAuth == nil {
		return nil
	}

	basicAuth := profiler.Spec.ScrapTarget.Auth.BasicAuth

	if basicAuth.Username != "" && basicAuth.Password != "" {
		return &crdAuth{
			directAuth: true,
			username:   basicAuth.Username,
			password:   basicAuth.Password,
		}
	}

	if basicAuth.SecretRef != nil && basicAuth.SecretRef.Name != "" {
		usernameKey := basicAuth.SecretRef.UsernameKey
		if usernameKey == "" {
			usernameKey = "username"
		}

		passwordKey := basicAuth.SecretRef.PasswordKey
		if passwordKey == "" {
			passwordKey = "password"
		}

		return &crdAuth{
			secretRef: &secretAuth{
				name:        basicAuth.SecretRef.Name,
				usernameKey: usernameKey,
				passwordKey: passwordKey,
			},
		}
	}

	return nil
}

func buildBasicAuthEnvVars(username, password string) []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name:  "AUTH_USERNAME",
			Value: username,
		},
		{
			Name:  "AUTH_PASSWORD",
			Value: password,
		},
	}
}

func buildSecretAuthEnvVars(secretName, usernameKey, passwordKey string) []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name: "AUTH_USERNAME",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: secretName,
					},
					Key: usernameKey,
				},
			},
		},
		{
			Name: "AUTH_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: secretName,
					},
					Key: passwordKey,
				},
			},
		},
	}
}

// getConfiguredMonitoringPeriod returns the monitoring period configuration as an environment variable
// It reads the monitoring period from the CRD
func getConfiguredMonitoringPeriod(profiler *observabilityv1.Profiler, logger logr.Logger) []corev1.EnvVar {
	logger.V(1).Info("Configuring monitoring period")

	var envVars []corev1.EnvVar

	if profiler.Spec.MonitoringPeriod != 0 {
		logger.Info("Using monitoring period from CRD",
			"value", profiler.Spec.MonitoringPeriod)
		envVars = append(envVars, corev1.EnvVar{
			Name:  "MONITORING_PERIOD",
			Value: fmt.Sprintf("%d", profiler.Spec.MonitoringPeriod),
		})
	} else {
		logger.V(1).Info("No monitoring period configured in CRD, using default")
	}

	return envVars
}
