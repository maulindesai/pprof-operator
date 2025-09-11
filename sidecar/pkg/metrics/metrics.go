package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// ProfilesGenerated tracks the number of pprof profiles generated
	ProfilesGenerated = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "pprof_sidecar_profiles_generated_total",
			Help: "Total number of pprof profiles generated",
		},
		[]string{"profile_type"},
	)

	// ProfilesUploaded tracks the number of pprof profiles uploaded to S3
	ProfilesUploaded = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "pprof_sidecar_profiles_uploaded_total",
			Help: "Total number of pprof profiles uploaded to S3",
		},
	)

	// ProfileUploadErrors tracks the number of errors when uploading profiles to S3
	ProfileUploadErrors = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "pprof_sidecar_profile_upload_errors_total",
			Help: "Total number of errors when uploading profiles to S3",
		},
	)

	// ThresholdExceeded tracks the number of times resource thresholds were exceeded
	ThresholdExceeded = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "pprof_sidecar_threshold_exceeded_total",
			Help: "Total number of times resource thresholds were exceeded",
		},
		[]string{"resource_type"},
	)

	// ResourceUsage tracks the current resource usage as a percentage
	ResourceUsage = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "pprof_sidecar_resource_usage_percent",
			Help: "Current resource usage as a percentage",
		},
		[]string{"resource_type"},
	)
)

// Registry is a custom registry for sidecar metrics
var Registry = prometheus.NewRegistry()

func init() {
	// Register metrics with the custom registry
	Registry.MustRegister(
		ProfilesGenerated,
		ProfilesUploaded,
		ProfileUploadErrors,
		ThresholdExceeded,
		ResourceUsage,
	)
}

// Handler returns an HTTP handler for the metrics endpoint
func Handler() http.Handler {
	return promhttp.HandlerFor(Registry, promhttp.HandlerOpts{})
}

// StartMetricsServer starts a metrics server on the specified address
func StartMetricsServer(addr string) error {
	http.Handle("/metrics", Handler())
	return http.ListenAndServe(addr, nil)
}