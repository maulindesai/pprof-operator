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
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	observabilityv1 "github.com/maulindesai/pprof-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var podlog = logf.Log.WithName("pod-webhook")

// +kubebuilder:webhook:path=/mutate-v1-pod,mutating=true,failurePolicy=ignore,sideEffects=None,groups=core,resources=pods,verbs=create,versions=v1,name=mpod-v1.kb.io,admissionReviewVersions=v1

// PodMutator mutates pods to inject sidecar containers
type PodMutator struct {
	Client  client.Client
	decoder *admission.Decoder
}

// Handle implements admission.Handler.
func (m *PodMutator) Handle(ctx context.Context, req admission.Request) admission.Response {

	pod := &corev1.Pod{}

	if m.decoder == nil {
		err := fmt.Errorf("decoder is not initialized")
		podlog.Error(err, "Decoder is nil")
		return admission.Errored(http.StatusInternalServerError, err)
	}

	err := (*m.decoder).Decode(req, pod)
	if err != nil {
		podlog.Error(err, "Failed to decode pod")
		return admission.Errored(http.StatusBadRequest, err)
	}

	value, exists := pod.Annotations[AnnotationProfilerEnable]
	if !exists {
		podlog.Info("Pod does not have profiler.pprof.dev/enable annotation", "pod", pod.Name)
		return admission.Allowed("Pod does not have profiler.pprof.dev/enable annotation")
	}

	if value == "false" {
		if HasSidecar(pod.Spec.Containers, "pprof-sidecar") {
			podlog.Info("Pod does not have profiler.pprof.dev/enable=true annotation", "pod", pod.Name)
			podlog.Info("Removing pprof-sidecar", "pod", pod.Name)
			return m.removeSidecarIfExists(ctx, pod, req)
		}
	}
	// Get the profiler resource from the same namespace
	profilerList := &observabilityv1.ProfilerList{}
	if err := m.Client.List(ctx, profilerList, client.InNamespace(req.Namespace)); err != nil {
		podlog.Error(err, "Failed to list profilers")
		return admission.Errored(http.StatusInternalServerError, err)
	}

	// If no profiler found, allow the pod without modification
	if len(profilerList.Items) == 0 {
		podlog.Info("No profiler found in namespace", "namespace", req.Namespace)
		return admission.Allowed("No profiler found in namespace")
	}

	// Use the first profiler found
	profiler := &profilerList.Items[0]

	// Build the sidecar container
	sidecar, err := BuildProfilerSidecar(profiler, pod)
	if err != nil {
		podlog.Error(err, "Failed to build profiler sidecar")
		return admission.Errored(http.StatusInternalServerError, err)
	}

	// Add the sidecar container to the pod
	pod.Spec.Containers = append(pod.Spec.Containers, *sidecar)

	// Create the patched pod
	marshaledPod, err := json.Marshal(pod)
	if err != nil {
		podlog.Error(err, "Failed to marshal patched pod")
		return admission.Errored(http.StatusInternalServerError, err)
	}
	// Create the patch response
	return admission.PatchResponseFromRaw(req.Object.Raw, marshaledPod)
}

// InjectDecoder injects the decoder into the PodMutator
func (m *PodMutator) InjectDecoder(d *admission.Decoder) error {
	podlog.Info("InjectDecoder called.")
	m.decoder = d
	return nil
}

// removeSidecarIfExists removes the profiler sidecar container from the pod if it exists
// It also removes the cgroup volume if no other containers are using it
func (r *PodMutator) removeSidecarIfExists(ctx context.Context, pod *corev1.Pod, req admission.Request) admission.Response {
	logger := logf.FromContext(ctx).WithValues("Pod", pod.Name, "Namespace", pod.Namespace)
	logger.V(1).Info("Checking if profiler sidecar needs to be removed")

	// Check if the pod has the sidecar container using the existing helper function
	if !HasSidecar(pod.Spec.Containers, "pprof-sidecar") {
		logger.Info("Pod does not have profiler sidecar, no action needed")
		return admission.Allowed("Pod does not have profiler sidecar")
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

	// Apply the patch
	logger.V(1).Info("Applying patch to remove profiler sidecar")
	marshaledPod, err := json.Marshal(patched)
	if err != nil {
		logger.Error(err, "Failed to marshal patched pod")
		return admission.Errored(http.StatusInternalServerError, err)
	}
	return admission.PatchResponseFromRaw(req.Object.Raw, marshaledPod)
}

// SetupPodWebhookWithManager registers the webhook for Pod in the manager.
func SetupPodWebhookWithManager(mgr ctrl.Manager) error {
	podMutator := &PodMutator{
		Client: mgr.GetClient(),
	}

	// Create a decoder for the webhook
	decoder := admission.NewDecoder(mgr.GetScheme())

	// Inject the decoder into the pod mutator
	if err := podMutator.InjectDecoder(&decoder); err != nil {
		return fmt.Errorf("failed to inject decoder: %w", err)
	}

	// Create a webhook with the manager's scheme
	webhook := &admission.Webhook{
		Handler: podMutator,
	}

	// Register the webhook
	mgr.GetWebhookServer().Register("/mutate-v1-pod", webhook)

	return nil
}
