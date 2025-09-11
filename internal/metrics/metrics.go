package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// SidecarsAttached tracks the number of pprof sidecars attached to pods
	SidecarsAttached = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "pprof_operator_sidecars_attached_total",
			Help: "Total number of pprof sidecars attached to pods",
		},
	)

	// SidecarsRemoved tracks the number of pprof sidecars removed from pods
	SidecarsRemoved = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "pprof_operator_sidecars_removed_total",
			Help: "Total number of pprof sidecars removed from pods",
		},
	)

	// ActiveSidecars tracks the current number of active pprof sidecars
	ActiveSidecars = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "pprof_operator_active_sidecars",
			Help: "Current number of active pprof sidecars",
		},
	)

	// ProfilesGenerated tracks the number of pprof profiles generated
	ProfilesGenerated = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "pprof_operator_profiles_generated_total",
			Help: "Total number of pprof profiles generated",
		},
		[]string{"profile_type"},
	)

	// ProfilesUploaded tracks the number of pprof profiles uploaded to S3
	ProfilesUploaded = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "pprof_operator_profiles_uploaded_total",
			Help: "Total number of pprof profiles uploaded to S3",
		},
	)

	// ProfileUploadErrors tracks the number of errors when uploading profiles to S3
	ProfileUploadErrors = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "pprof_operator_profile_upload_errors_total",
			Help: "Total number of errors when uploading profiles to S3",
		},
	)

	// ThresholdExceeded tracks the number of times resource thresholds were exceeded
	ThresholdExceeded = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "pprof_operator_threshold_exceeded_total",
			Help: "Total number of times resource thresholds were exceeded",
		},
		[]string{"resource_type"},
	)
)

func init() {
	// Register custom metrics with the global prometheus registry
	metrics.Registry.MustRegister(
		SidecarsAttached,
		SidecarsRemoved,
		ActiveSidecars,
		ProfilesGenerated,
		ProfilesUploaded,
		ProfileUploadErrors,
		ThresholdExceeded,
	)
}
