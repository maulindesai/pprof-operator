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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"os"
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
	// Use environment variable for sidecar image if set, otherwise use default
	sidecarImage := "ghcr.io/maulindesai/pprof-operator/pprof-sidecar:latest"
	if envImage := os.Getenv("PPROF_SIDECAR_IMAGE"); envImage != "" {
		sidecarImage = envImage
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

	logger.V(1).Info("Finished building profiler sidecar container")

	return &sidecarContainer, nil
}
