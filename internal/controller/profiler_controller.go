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

package controller

import (
	"context"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;update;patch;update;delete
// +kubebuilder:rbac:groups=core,resources=pods/status,verbs=get
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch

// Reconcile handles the reconciliation loop for Profiler resources
// The actual pod mutation is handled by webhooks, so this controller just manages the Profiler resource
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *ProfilerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx).WithValues("ProfilerName", req.Name, "Namespace", req.Namespace)
	logger.Info("Starting reconciliation")

	// Fetch the Profiler instance
	profiler := &observabilityv1.Profiler{}
	if err := r.Get(ctx, req.NamespacedName, profiler); err != nil {
		return r.handleProfilerGetError(ctx, err)
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

// SetupWithManager sets up the controller with the Manager.
func (r *ProfilerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// No need for indexing as we're using webhooks for pod mutation

	return ctrl.NewControllerManagedBy(mgr).
		For(&observabilityv1.Profiler{}).
		Owns(&appsv1.Deployment{}).
		Owns(&appsv1.StatefulSet{}).
		Named("profiler").
		Complete(r)
}
