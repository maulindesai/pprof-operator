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

package v1

import (
	"fmt"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	observabilityv1 "github.com/maulindesai/pprof-operator/api/v1"
)

// BuildProfilerSidecar creates a sidecar container for profiling based on the profiler configuration
func BuildProfilerSidecar(profiler *observabilityv1.Profiler, pod *corev1.Pod) (*corev1.Container, error) {
	logger := logf.Log.WithName("buildProfilerSidecar").
		WithValues("pod", pod.Name, "namespace", pod.Namespace)
	logger.V(1).Info("Building profiler sidecar container")

	// Determine a target container-use the first container in the pod if not specified in annotation
	var targetContainer string
	targetContainerAnnotation, exists := pod.Annotations[AnnotationTargetContainer]

	if exists && targetContainerAnnotation != "" {
		logger.Info("Using target container from annotation", "container", targetContainerAnnotation)
		targetContainer = targetContainerAnnotation
	} else if len(pod.Spec.Containers) > 0 {
		targetContainer = pod.Spec.Containers[0].Name
		logger.Info("No target container specified in annotation, using first container", "container", targetContainer)
	} else {
		return nil, fmt.Errorf("no containers found in pod %s/%s", pod.Namespace, pod.Name)
	}

	// Create the base container with standard configuration
	// Determine sidecar image: annotation > default
	sidecarImage := "ghcr.io/maulindesai/pprof-operator/pprof-sidecar:latest"
	if annImage, ok := pod.Annotations[AnnotationSidecarImage]; ok && strings.TrimSpace(annImage) != "" {
		logger.Info("Overriding sidecar image from annotation", "annotation", AnnotationSidecarImage, "image", annImage)
		sidecarImage = strings.TrimSpace(annImage)
	}

	sidecarContainer := corev1.Container{
		Name:            "pprof-sidecar",
		Image:           sidecarImage,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Env: []corev1.EnvVar{
			{
				Name:  "TARGET_CONTAINER",
				Value: targetContainer,
			},
			{
				Name: "POD_NAME",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "metadata.name",
					},
				},
			},
			{
				Name: "POD_NAMESPACE",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "metadata.namespace",
					},
				},
			},
		},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("128Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("200m"),
				corev1.ResourceMemory: resource.MustParse("256Mi"),
			},
		},
	}

	// Add CPU threshold configuration from CRD
	cpuThresholdEnvVars := getConfiguredCPUThreshold(profiler, logger)
	sidecarContainer.Env = append(sidecarContainer.Env, cpuThresholdEnvVars...)

	// Add memory threshold configuration from CRD
	memoryThresholdEnvVars := getConfiguredMemoryThreshold(profiler, logger)
	sidecarContainer.Env = append(sidecarContainer.Env, memoryThresholdEnvVars...)

	// Add monitoring period from CRD if specified
	monitoringPeriodEnvVars := getConfiguredMonitoringPeriod(profiler, logger)
	sidecarContainer.Env = append(sidecarContainer.Env, monitoringPeriodEnvVars...)

	// Add AWS configuration from CRD
	awsConfigEnvVars := getAWSConfiguration(profiler, logger)
	sidecarContainer.Env = append(sidecarContainer.Env, awsConfigEnvVars...)

	// Add scrape target configuration from CRD
	scrapConfigEnvVars := getScrapeTargetConfiguration(profiler, pod, logger)
	sidecarContainer.Env = append(sidecarContainer.Env, scrapConfigEnvVars...)

	// Optional: Custom S3-compatible endpoint via pod annotations (e.g., DigitalOcean Spaces)
	if ep, ok := pod.Annotations[AnnotationS3Endpoint]; ok && ep != "" {
		logger.Info("Using custom S3-compatible endpoint from annotation", "endpoint", ep)
		sidecarContainer.Env = append(sidecarContainer.Env, corev1.EnvVar{
			Name:  "S3_ENDPOINT",
			Value: ep,
		})
	}
	if fps, ok := pod.Annotations[AnnotationS3ForcePathStyle]; ok && fps != "" {
		logger.Info("Using S3 force path style from annotation", "value", fps)
		sidecarContainer.Env = append(sidecarContainer.Env, corev1.EnvVar{
			Name:  "S3_FORCE_PATH_STYLE",
			Value: fps,
		})
	}

	// Enable debug logging if requested via annotation
	if dbg, ok := pod.Annotations[AnnotationDebug]; ok {
		v := strings.ToLower(strings.TrimSpace(dbg))
		if v == "true" || v == "1" || v == "debug" {
			logger.Info("Enabling debug logging for sidecar via annotation", "annotation", AnnotationDebug, "value", dbg)
			sidecarContainer.Env = append(sidecarContainer.Env, corev1.EnvVar{Name: "LOG_LEVEL", Value: "debug"})
			// Backward compatibility env var
			sidecarContainer.Env = append(sidecarContainer.Env, corev1.EnvVar{Name: "DEBUG", Value: "true"})
		}
	}

	// Configure metrics port and env (default 8080, override via annotation)
	metricsPort := 8080
	if annPort, ok := pod.Annotations[AnnotationSidecarMetricsPort]; ok && strings.TrimSpace(annPort) != "" {
		p, err := strconv.Atoi(strings.TrimSpace(annPort))
		if err != nil || p <= 0 || p > 65535 {
			logger.Error(fmt.Errorf("invalid metrics port: %s", annPort), "Invalid metrics port in annotation, falling back to default", "annotation", AnnotationSidecarMetricsPort)
		} else {
			metricsPort = p
		}
	}
	// Expose the container port and set METRICS_ADDR for the sidecar process
	sidecarContainer.Ports = append(sidecarContainer.Ports, corev1.ContainerPort{
		Name:          "metrics",
		ContainerPort: int32(metricsPort),
		Protocol:      corev1.ProtocolTCP,
	})
	sidecarContainer.Env = append(sidecarContainer.Env, corev1.EnvVar{
		Name:  "METRICS_ADDR",
		Value: fmt.Sprintf(":%d", metricsPort),
	})

	logger.V(1).Info("Finished building profiler sidecar container")

	return &sidecarContainer, nil
}
