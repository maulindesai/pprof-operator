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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"time"

	observabilityv1 "github.com/maulindesai/pprof-operator/api/v1"
)

// ProfilerReconciler reconciles a Profiler object
type ProfilerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=observability.pprof-operator.dev,resources=profilers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=observability.pprof-operator.dev,resources=profilers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=observability.pprof-operator.dev,resources=profilers/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=core,resources=pods/status,verbs=get
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch

// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *ProfilerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger.Info("Reconciling Profiler", "request", req.NamespacedName)

	// Fetch the Profiler instance
	profiler := &observabilityv1.Profiler{}
	err := r.Get(ctx, req.NamespacedName, profiler)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Return and don't requeue
			logger.Info("Profiler resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		logger.Error(err, "Failed to get Profiler")
		return ctrl.Result{}, err
	}

	// Find all pods matching the selector in the same namespace
	podList := &corev1.PodList{}
	listOpts := []client.ListOption{
		client.InNamespace(req.Namespace),
	}

	// If selector is specified, use it to find pods
	if profiler.Spec.Selector != nil && profiler.Spec.Selector.MatchLabels != nil {
		listOpts = append(listOpts, client.MatchingLabels(profiler.Spec.Selector.MatchLabels))
		logger.Info("Using selector to find pods", "MatchLabels", profiler.Spec.Selector.MatchLabels)
	} else {
		// Fallback to annotation-based selection for backward compatibility
		logger.Info("No selector specified, using annotation-based selection")
	}

	if err = r.List(ctx, podList, listOpts...); err != nil {
		logger.Error(err, "Failed to list pods", "Namespace", req.Namespace)
		return ctrl.Result{}, err
	}

	// Process each pod to ensure it has the profiler sidecar if needed
	for _, pod := range podList.Items {
		// If using selector, all pods in the list match the criteria
		// If using annotation-based selection, check for the annotation
		if profiler.Spec.Selector != nil && profiler.Spec.Selector.MatchLabels != nil {
			if err := r.ensureProfilerSidecar(ctx, &pod, profiler); err != nil {
				logger.Error(err, "Failed to ensure profiler sidecar", "Pod", pod.Name)
				return ctrl.Result{}, err
			}
		} else if value, exists := pod.Annotations["profiler.pprof.dev/enable"]; exists && value == "true" {
			if err := r.ensureProfilerSidecar(ctx, &pod, profiler); err != nil {
				logger.Error(err, "Failed to ensure profiler sidecar", "Pod", pod.Name)
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
}

// ensureProfilerSidecar ensures that the pod has the profiler sidecar container
func (r *ProfilerReconciler) ensureProfilerSidecar(ctx context.Context, pod *corev1.Pod, profiler *observabilityv1.Profiler) error {
	logger := logf.FromContext(ctx)

	// Check if the pod already has the sidecar container
	for _, container := range pod.Spec.Containers {
		if container.Name == "pprof-sidecar" {
			logger.Info("Pod already has profiler sidecar", "Pod", pod.Name)
			return nil
		}
	}

	// Find the owning deployment or statefulset
	ownerRef := metav1.GetControllerOf(pod)
	if ownerRef == nil {
		logger.Info("Pod has no owner reference", "Pod", pod.Name)
		return nil
	}

	//var patchTarget client.Object
	switch ownerRef.Kind {
	case "Deployment":
		deployment := &appsv1.Deployment{}
		if err := r.Get(ctx, client.ObjectKey{Namespace: pod.Namespace, Name: ownerRef.Name}, deployment); err != nil {
			logger.Error(err, "Failed to get deployment", "Name", ownerRef.Name)
			return err
		}
		sidecar := buildProfilerSidecar(profiler, pod)
		if err := r.patchDeploymentWithSidecar(ctx, deployment, sidecar); err != nil {
			logger.Error(err, "Failed to patch deployment with sidecar", "Name", ownerRef.Name)
			return err
		}

	case "StatefulSet":
		statefulset := &appsv1.StatefulSet{}
		if err := r.Get(ctx, client.ObjectKey{Namespace: pod.Namespace, Name: ownerRef.Name}, statefulset); err != nil {
			logger.Error(err, "Failed to get statefulset", "Name", ownerRef.Name)
			return err
		}
		sidecar := buildProfilerSidecar(profiler, pod)
		if err := r.patchStatefulSetWithSidecar(ctx, statefulset, sidecar); err != nil {
			logger.Error(err, "Failed to patch statefulset with sidecar", "Name", ownerRef.Name)
			return err
		}
	default:
		logger.Info("Pod owner is not deployment or statefulset", "Owner Kind", ownerRef.Kind)
		return nil
	}

	logger.Info("Added profiler sidecar to workload",
		"Kind", ownerRef.Kind,
		"Name", ownerRef.Name,
		"TargetContainer", profiler.Spec.TargetContainer)

	return nil
}

func (r *ProfilerReconciler) patchDeploymentWithSidecar(ctx context.Context, deployment *appsv1.Deployment, sidecar corev1.Container) error {
	patched := deployment.DeepCopy()
	// Check idempotency
	if hasSidecar(patched.Spec.Template.Spec.Containers, sidecar.Name) {
		return nil
	}
	patched.Spec.Template.Spec.Containers = append(patched.Spec.Template.Spec.Containers, sidecar)
	patch := client.MergeFrom(deployment)
	return r.Patch(ctx, patched, patch)
}

func (r *ProfilerReconciler) patchStatefulSetWithSidecar(ctx context.Context, sts *appsv1.StatefulSet, sidecar corev1.Container) error {
	patched := sts.DeepCopy()
	if hasSidecar(patched.Spec.Template.Spec.Containers, sidecar.Name) {
		return nil
	}
	patched.Spec.Template.Spec.Containers = append(patched.Spec.Template.Spec.Containers, sidecar)
	patch := client.MergeFrom(sts)
	return r.Patch(ctx, patched, patch)
}

func hasSidecar(containers []corev1.Container, name string) bool {
	for _, container := range containers {
		if container.Name == name {
			return true
		}
	}
	return false
}

// build the sidecar container
func buildProfilerSidecar(profiler *observabilityv1.Profiler, pod *corev1.Pod) corev1.Container {
	logger := logf.Log.WithName("buildProfilerSidecar").
		WithValues("pod", pod.Name, "namespace", pod.Namespace)

	sidecarContainer := corev1.Container{
		Name:  "pprof-sidecar",
		Image: "pprof-operator/pprof-sidecar:latest",
		Env: []corev1.EnvVar{
			{
				Name:  "TARGET_CONTAINER",
				Value: profiler.Spec.TargetContainer,
			},
			{
				Name:  "TARGET_PID",
				Value: "1", // This is a simplification, in reality we would need to find the PID
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

	// Check for CPU threshold annotation first, then fall back to CRD value
	cpuThresholdAnnotation, hasCPUAnnotation := pod.Annotations["profiler.pprof.dev/cpu-threshold"]
	if hasCPUAnnotation {
		logger.Info("Using CPU threshold from annotation",
			"value", cpuThresholdAnnotation,
			"annotation", "profiler.pprof.dev/cpu-threshold")
		sidecarContainer.Env = append(sidecarContainer.Env, corev1.EnvVar{
			Name:  "CPU_THRESHOLD",
			Value: cpuThresholdAnnotation,
		})
	} else if profiler.Spec.CPUThreshold != nil {
		logger.Info("Using CPU threshold from CRD",
			"value", *profiler.Spec.CPUThreshold)
		sidecarContainer.Env = append(sidecarContainer.Env, corev1.EnvVar{
			Name:  "CPU_THRESHOLD",
			Value: fmt.Sprintf("%d", *profiler.Spec.CPUThreshold),
		})
	}

	// Check for memory threshold annotation first, then fall back to CRD value
	memoryThresholdAnnotation, hasMemoryAnnotation := pod.Annotations["profiler.pprof.dev/memory-threshold"]
	if hasMemoryAnnotation {
		logger.Info("Using memory threshold from annotation",
			"value", memoryThresholdAnnotation,
			"annotation", "profiler.pprof.dev/memory-threshold")
		sidecarContainer.Env = append(sidecarContainer.Env, corev1.EnvVar{
			Name:  "MEMORY_THRESHOLD",
			Value: memoryThresholdAnnotation,
		})
	} else if profiler.Spec.MemoryThreshold != nil {
		logger.Info("Using memory threshold from CRD",
			"value", *profiler.Spec.MemoryThreshold)
		sidecarContainer.Env = append(sidecarContainer.Env, corev1.EnvVar{
			Name:  "MEMORY_THRESHOLD",
			Value: fmt.Sprintf("%d", *profiler.Spec.MemoryThreshold),
		})
	}

	// Add profile duration if specified
	if profiler.Spec.ProfileDuration != 0 {
		sidecarContainer.Env = append(sidecarContainer.Env, corev1.EnvVar{
			Name:  "PROFILE_DURATION",
			Value: fmt.Sprintf("%d", profiler.Spec.ProfileDuration),
		})
	}

	// Add S3 configuration
	sidecarContainer.Env = append(sidecarContainer.Env, []corev1.EnvVar{
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
	}...)

	// Add AWS credentials from secret if specified
	if profiler.Spec.AWSCredentialsSecret != "" {
		sidecarContainer.Env = append(sidecarContainer.Env, []corev1.EnvVar{
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
	}
	return sidecarContainer
}

// SetupWithManager sets up the controller with the Manager.
func (r *ProfilerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	err := mgr.GetFieldIndexer().IndexField(context.Background(), &corev1.Pod{}, "profiler.pprof.dev/enable", func(rawObj client.Object) []string {
		pod := rawObj.(*corev1.Pod)
		value, exists := pod.Annotations["profiler.pprof.dev/enable"]
		if !exists {
			return nil
		}
		return []string{value}
	})
	if err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&observabilityv1.Profiler{}).
		Owns(&appsv1.Deployment{}).
		Owns(&appsv1.StatefulSet{}).
		Named("profiler").
		Complete(r)
}
