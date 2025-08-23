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
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"time"

	observabilityv1 "github.com/maulindesai/pprof-operator/api/v1"
)

const (
	// AnnotationProfilerEnable General annotations
	// AnnotationProfilerEnable is the annotation key used to enable/disable the profiler sidecar
	AnnotationProfilerEnable = "profiler.pprof.dev/enable"
	// AnnotationTargetContainer is the annotation key used to specify the target container for profiling
	AnnotationTargetContainer = "profiler.pprof.dev/target-container"

	// Scraping annotations
	// AnnotationScrapURL is the annotation key used to specify the URL to scrape for profiling data
	AnnotationScrapURL = "profiler.pprof.dev/scrap/url"
	// AnnotationScrapAuthType is the annotation key used to specify the authentication type for scraping
	AnnotationScrapAuthType = "profiler.pprof.dev/scrap/auth-type"
	// AnnotationScrapAuthUsername is the annotation key used to specify the username for scraping authentication
	AnnotationScrapAuthUsername = "profiler.pprof.dev/scrap/auth-username"
	// AnnotationScrapAuthPassword is the annotation key used to specify the password for scraping authentication
	AnnotationScrapAuthPassword = "profiler.pprof.dev/scrap/auth-password"
	// AnnotationScrapAuthSecret is the annotation key used to specify the secret for scraping authentication
	AnnotationScrapAuthSecret = "profiler.pprof.dev/scrap/auth-secret"
	// AnnotationScrapAuthUsernameKey is the annotation key used to specify the username key in the secret
	AnnotationScrapAuthUsernameKey = "profiler.pprof.dev/scrap/auth-username-key"
	// AnnotationScrapAuthPasswordKey is the annotation key used to specify the password key in the secret
	AnnotationScrapAuthPasswordKey = "profiler.pprof.dev/scrap/auth-password-key"
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
	if err := r.Get(ctx, req.NamespacedName, profiler); err != nil {
		return r.handleProfilerGetError(ctx, err)
	}

	// Find pods that need to be processed
	podList, err := r.findPodsToProcess(ctx, req.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Process each pod based on profiler configuration and annotations
	if err := r.processPods(ctx, podList, profiler); err != nil {
		return ctrl.Result{}, err
	}

	// Requeue after 5 minutes for periodic reconciliation
	return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
}

// handleProfilerGetError handles errors when getting the Profiler resource
func (r *ProfilerReconciler) handleProfilerGetError(ctx context.Context, err error) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

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

// findPodsToProcess finds all pods that need to be processed based on selector or annotations
func (r *ProfilerReconciler) findPodsToProcess(ctx context.Context, namespace string) (*corev1.PodList, error) {
	logger := logf.FromContext(ctx)

	podList := &corev1.PodList{}
	listOpts := []client.ListOption{
		client.InNamespace(namespace),
	}

	if err := r.List(ctx, podList, listOpts...); err != nil {
		logger.Error(err, "Failed to list pods", "Namespace", namespace)
		return nil, err
	}

	return podList, nil
}

// processPods processes each pod to ensure it has the profiler sidecar if needed or removed if disabled
func (r *ProfilerReconciler) processPods(ctx context.Context, podList *corev1.PodList, profiler *observabilityv1.Profiler) error {
	logger := logf.FromContext(ctx)

	for i := range podList.Items {
		pod := &podList.Items[i]

		// Check if the pod has the annotation and get its value
		value, exists := pod.Annotations[AnnotationProfilerEnable]

		if exists {
			// Handle based on annotation value
			if value == "true" {
				// Add sidecar if annotation is true
				logger.Info("Processing pod with annotation=true", "Pod", pod.Name)
				if err := r.ensureProfilerSidecar(ctx, pod, profiler); err != nil {
					logger.Error(err, "Failed to ensure profiler sidecar", "Pod", pod.Name)
					return err
				}
			} else if value == "false" {
				// Remove sidecar if annotation is false
				logger.Info("Processing pod with annotation=false", "Pod", pod.Name)
				if err := r.removeSidecarIfExists(ctx, pod); err != nil {
					logger.Error(err, "Failed to remove profiler sidecar", "Pod", pod.Name)
					return err
				}
			} else {
				logger.Info("Skipping pod with invalid annotation value", "Pod", pod.Name, "Value", value)
			}
		} else {
			logger.V(1).Info("Skipping pod without annotation", "Pod", pod.Name)
		}
	}

	return nil
}

// ensureProfilerSidecar ensures that the pod has the profiler sidecar container
// It checks if the sidecar already exists, and if not, adds it to the pod
func (r *ProfilerReconciler) ensureProfilerSidecar(ctx context.Context, pod *corev1.Pod, profiler *observabilityv1.Profiler) error {
	logger := logf.FromContext(ctx).WithValues("Pod", pod.Name, "Namespace", pod.Namespace)
	logger.V(1).Info("Ensuring profiler sidecar exists")

	// Check if the pod already has the sidecar container
	if hasSidecar(pod.Spec.Containers, "pprof-sidecar") {
		logger.Info("Pod already has profiler sidecar")
		return nil
	}

	// Build and add the sidecar container
	logger.V(1).Info("Building profiler sidecar container")
	sidecar, err := buildProfilerSidecar(profiler, pod)
	if err != nil {
		logger.Error(err, "Failed to build profiler sidecar container")
		return fmt.Errorf("failed to build profiler sidecar container for pod %s: %w", pod.Name, err)
	}

	logger.V(1).Info("Patching pod with sidecar container")
	if err := r.patchPodWithSidecar(ctx, pod, *sidecar); err != nil {
		logger.Error(err, "Failed to patch pod with sidecar")
		return fmt.Errorf("failed to patch pod %s with sidecar: %w", pod.Name, err)
	}

	logger.Info("Successfully added profiler sidecar to pod")
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ProfilerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	err := mgr.GetFieldIndexer().IndexField(context.Background(), &corev1.Pod{}, AnnotationProfilerEnable, func(rawObj client.Object) []string {
		pod := rawObj.(*corev1.Pod)
		value, exists := pod.Annotations[AnnotationProfilerEnable]
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
