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
	"context"
	"fmt"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	observabilityv1 "github.com/maulindesai/pprof-operator/api/v1"
)

// nolint:unused
// log is for logging in this package.
var profilerlog = logf.Log.WithName("profiler-resource")

// SetupProfilerWebhookWithManager registers the webhook for Profiler in the manager.
func SetupProfilerWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&observabilityv1.Profiler{}).
		WithValidator(&ProfilerCustomValidator{}).
		WithDefaulter(&ProfilerCustomDefaulter{}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-observability-pprof-operator-dev-v1-profiler,mutating=true,failurePolicy=fail,sideEffects=None,groups=observability.pprof-operator.dev,resources=profilers,verbs=create;update,versions=v1,name=mprofiler-v1.kb.io,admissionReviewVersions=v1

// ProfilerCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind Profiler when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type ProfilerCustomDefaulter struct{}

var _ webhook.CustomDefaulter = &ProfilerCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind Profiler.
func (d *ProfilerCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	profiler, ok := obj.(*observabilityv1.Profiler)

	if !ok {
		return fmt.Errorf("expected an Profiler object but got %T", obj)
	}
	profilerlog.Info("Defaulting for Profiler", "name", profiler.GetName())

	// Set default values for fields that aren't specified
	if profiler.Spec.CPUThreshold == nil && profiler.Spec.MemoryThreshold == nil {
		defaultValue := int32(80)
		profiler.Spec.CPUThreshold = &defaultValue
		profiler.Spec.MemoryThreshold = &defaultValue
		profilerlog.Info("defaulting thresholds", "cpuThreshold", defaultValue, "memoryThreshold", defaultValue)
	}
	// Default S3 path prefix if not specified
	if profiler.Spec.S3PathPrefix == "" {
		profiler.Spec.S3PathPrefix = "profiles"
		profilerlog.Info("defaulting s3PathPrefix", "value", profiler.Spec.S3PathPrefix)
	}

	// Note: ProfileDuration and S3Region already have defaults set in the CRD definition
	// using kubebuilder markers, so we don't need to set them here
	return nil
}

// +kubebuilder:webhook:path=/validate-observability-pprof-operator-dev-v1-profiler,mutating=false,failurePolicy=fail,sideEffects=None,groups=observability.pprof-operator.dev,resources=profilers,verbs=create;update,versions=v1,name=vprofiler-v1.kb.io,admissionReviewVersions=v1

// ProfilerCustomValidator struct is responsible for validating the Profiler resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type ProfilerCustomValidator struct {
}

var _ webhook.CustomValidator = &ProfilerCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type Profiler.
func (v *ProfilerCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	profiler, ok := obj.(*observabilityv1.Profiler)
	if !ok {
		return nil, fmt.Errorf("expected a Profiler object but got %T", obj)
	}
	profilerlog.Info("Validation for Profiler upon creation", "name", profiler.GetName())

	return v.validateProfiler(profiler)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type Profiler.
func (v *ProfilerCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	profiler, ok := newObj.(*observabilityv1.Profiler)
	if !ok {
		return nil, fmt.Errorf("expected a Profiler object for the newObj but got %T", newObj)
	}
	profilerlog.Info("Validation for Profiler upon update", "name", profiler.GetName())

	return v.validateProfiler(profiler)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type Profiler.
func (v *ProfilerCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	profiler, ok := obj.(*observabilityv1.Profiler)
	if !ok {
		return nil, fmt.Errorf("expected a Profiler object but got %T", obj)
	}
	profilerlog.Info("Validation for Profiler upon deletion", "name", profiler.GetName())

	return nil, nil
}

// validateProfiler validates the Profiler resource
func (v *ProfilerCustomValidator) validateProfiler(profiler *observabilityv1.Profiler) (admission.Warnings, error) {
	var allErrs field.ErrorList
	var warnings admission.Warnings

	// Validate that at least one of CPU or Memory threshold is specified
	if profiler.Spec.CPUThreshold == nil && profiler.Spec.MemoryThreshold == nil {
		allErrs = append(allErrs, field.Required(field.NewPath("spec").Child("cpuThreshold"),
			"at least one of cpuThreshold or memoryThreshold must be specified"))
	}

	// Validate that target container is specified
	if profiler.Spec.TargetContainer == "" {
		allErrs = append(allErrs, field.Required(field.NewPath("spec").Child("targetContainer"),
			"targetContainer must be specified"))
	}

	// Validate that S3 bucket is specified
	if profiler.Spec.S3Bucket == "" {
		allErrs = append(allErrs, field.Required(field.NewPath("spec").Child("s3Bucket"),
			"s3Bucket must be specified"))
	}

	// Validate profile duration is reasonable
	if profiler.Spec.ProfileDuration > 300 {
		warnings = append(warnings, fmt.Sprintf("profileDuration is very long (%d seconds), which may impact performance",
			profiler.Spec.ProfileDuration))
	}

	if len(allErrs) == 0 {
		return warnings, nil
	}

	return warnings, allErrs.ToAggregate()
}
