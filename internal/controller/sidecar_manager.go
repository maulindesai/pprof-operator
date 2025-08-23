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

package controller

import (
	"context"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	observabilityv1 "github.com/maulindesai/pprof-operator/api/v1"
)

// removeSidecarIfExists removes the profiler sidecar container from the pod if it exists
// It also removes the cgroup volume if no other containers are using it
func (r *ProfilerReconciler) removeSidecarIfExists(ctx context.Context, pod *corev1.Pod) error {
	logger := logf.FromContext(ctx).WithValues("Pod", pod.Name, "Namespace", pod.Namespace)
	logger.V(1).Info("Checking if profiler sidecar needs to be removed")

	// Check if the pod has the sidecar container using the existing helper function
	if !hasSidecar(pod.Spec.Containers, "pprof-sidecar") {
		logger.Info("Pod does not have profiler sidecar, no action needed")
		return nil
	}

	logger.V(1).Info("Preparing to remove profiler sidecar")

	// Create a patched pod without the sidecar
	patched := pod.DeepCopy()

	// Remove the sidecar container
	var updatedContainers []corev1.Container
	for _, container := range patched.Spec.Containers {
		if container.Name != "pprof-sidecar" {
			updatedContainers = append(updatedContainers, container)
		}
	}
	patched.Spec.Containers = updatedContainers
	logger.V(1).Info("Removed sidecar container from pod spec")

	// Check if we need to remove the cgroup volume
	// Only remove if no other containers are using it
	if !isCgroupVolumeNeeded(patched) {
		logger.V(1).Info("Cgroup volume is no longer needed, removing it")
		patched.Spec.Volumes = removeCgroupVolume(patched.Spec.Volumes)
	} else {
		logger.V(1).Info("Cgroup volume is still needed by other containers, keeping it")
	}

	// Apply the patch
	logger.V(1).Info("Applying patch to remove profiler sidecar")
	patch := client.MergeFrom(pod)
	if err := r.Patch(ctx, patched, patch); err != nil {
		logger.Error(err, "Failed to remove profiler sidecar from pod")
		return fmt.Errorf("failed to remove profiler sidecar from pod %s: %w", pod.Name, err)
	}

	logger.Info("Successfully removed profiler sidecar from pod")
	return nil
}

// patchPodWithSidecar patches a pod with a sidecar container and ensures required volumes exist
func (r *ProfilerReconciler) patchPodWithSidecar(ctx context.Context, pod *corev1.Pod, sidecar corev1.Container) error {
	logger := logf.FromContext(ctx).WithValues("Pod", pod.Name, "Namespace", pod.Namespace)
	logger.V(1).Info("Preparing to patch pod with sidecar container")

	// Create a copy of the pod to modify
	patched := pod.DeepCopy()

	// Check idempotency - if sidecar already exists, nothing to do
	if hasSidecar(patched.Spec.Containers, sidecar.Name) {
		logger.V(1).Info("Sidecar already exists in pod, skipping patch")
		return nil
	}

	// Ensure cgroup volume exists
	if !hasCgroupVolume(patched.Spec.Volumes) {
		logger.V(1).Info("Adding cgroup volume to pod")
		patched.Spec.Volumes = append(patched.Spec.Volumes, createCgroupVolume())
	} else {
		logger.V(1).Info("Cgroup volume already exists in pod")
	}

	// Add the sidecar container
	logger.V(1).Info("Adding sidecar container to pod")
	patched.Spec.Containers = append(patched.Spec.Containers, sidecar)

	// Apply the patch
	logger.V(1).Info("Applying patch to pod")
	patch := client.MergeFrom(pod)
	if err := r.Patch(ctx, patched, patch); err != nil {
		logger.Error(err, "Failed to patch pod with sidecar")
		return fmt.Errorf("failed to patch pod %s with sidecar: %w", pod.Name, err)
	}

	logger.V(1).Info("Successfully patched pod with sidecar")
	return nil
}

// buildProfilerSidecar creates a sidecar container for profiling based on the profiler configuration
func buildProfilerSidecar(profiler *observabilityv1.Profiler, pod *corev1.Pod) (*corev1.Container, error) {
	logger := logf.Log.WithName("buildProfilerSidecar").
		WithValues("pod", pod.Name, "namespace", pod.Namespace)
	logger.V(1).Info("Building profiler sidecar container")

	// Determine target container - use the first container in the pod if not specified in annotation
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
	sidecarContainer := corev1.Container{
		Name:  "pprof-sidecar",
		Image: "pprof-operator/pprof-sidecar:latest",
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
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "cgroup",
				MountPath: "/sys/fs/cgroup",
				ReadOnly:  true,
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

	// Add profile duration from CRD if specified
	if profiler.Spec.ProfileDuration != 0 {
		logger.V(1).Info("Configuring profile duration", "duration", profiler.Spec.ProfileDuration)
		sidecarContainer.Env = append(sidecarContainer.Env, corev1.EnvVar{
			Name:  "PROFILE_DURATION",
			Value: fmt.Sprintf("%d", profiler.Spec.ProfileDuration),
		})
	} else {
		logger.V(1).Info("Using default profile duration")
	}

	// Add monitoring period from CRD if specified
	monitoringPeriodEnvVars := getConfiguredMonitoringPeriod(profiler, logger)
	sidecarContainer.Env = append(sidecarContainer.Env, monitoringPeriodEnvVars...)

	// Add AWS configuration from CRD
	awsConfigEnvVars := getAWSConfiguration(profiler, logger)
	sidecarContainer.Env = append(sidecarContainer.Env, awsConfigEnvVars...)

	// Add scrape target configuration from CRD
	scrapConfigEnvVars := getScrapTargetConfiguration(profiler, pod, logger)
	sidecarContainer.Env = append(sidecarContainer.Env, scrapConfigEnvVars...)

	logger.V(1).Info("Finished building profiler sidecar container")

	return &sidecarContainer, nil
}
