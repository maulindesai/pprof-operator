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

// Annotation constants for profiler configuration
const (
	// AnnotationProfilerEnable is the annotation key used to enable/disable the profiler sidecar
	AnnotationProfilerEnable = "profiler.pprof.dev/enable"

	// AnnotationProfilerName is the annotation key used to specify the name of the profiler
	AnnotationProfilerName = "profiler.pprof.dev/name"

	// AnnotationTargetContainer is the annotation key used to specify the target container for profiling
	AnnotationTargetContainer = "profiler.pprof.dev/target_container"

	// AnnotationSidecarImage allows overriding the sidecar image per pod
	AnnotationSidecarImage = "profiler.pprof.dev/sidecar_image"

	// AnnotationScrapeURL Scraping annotations
	// AnnotationScrapeURL is the annotation key used to specify the URL to scrape for profiling data
	AnnotationScrapeURL = "profiler.pprof.dev/scrape_url"

	// AnnotationS3Endpoint S3/Storage annotations for sidecar configuration
	// AnnotationS3Endpoint is the annotation key used to specify a custom S3-compatible endpoint (e.g., DigitalOcean Spaces)
	AnnotationS3Endpoint = "profiler.pprof.dev/s3_endpoint"
	// AnnotationS3ForcePathStyle controls whether to use path-style addressing for S3-compatible endpoints
	AnnotationS3ForcePathStyle = "profiler.pprof.dev/s3_force_path_style"

	// AnnotationScrapeAuthType is the annotation key used to specify the authentication type for scraping
	AnnotationScrapeAuthType = "profiler.pprof.dev/scrape_auth_type"

	// AnnotationScrapeAuthUsername is the annotation key used to specify the username for scraping authentication
	AnnotationScrapeAuthUsername = "profiler.pprof.dev/scrape_auth_username"

	// AnnotationScrapeAuthPassword is the annotation key used to specify the password for scraping authentication
	AnnotationScrapeAuthPassword = "profiler.pprof.dev/scrape_auth_password"

	// AnnotationScrapeAuthSecret is the annotation key used to specify the secret for scraping authentication
	AnnotationScrapeAuthSecret = "profiler.pprof.dev/scrape_auth_secret"

	// AnnotationScrapeAuthUsernameKey is the annotation key used to specify the username key in the secret
	AnnotationScrapeAuthUsernameKey = "profiler.pprof.dev/scrape_auth_username_key"

	// AnnotationScrapeAuthPasswordKey is the annotation key used to specify the password key in the secret
	AnnotationScrapeAuthPasswordKey = "profiler.pprof.dev/scrape_auth_password_key"

	// AnnotationDebug enables debug logging for the sidecar when set to a truthy value
	// Supported values: "true", "1", "debug" (case-insensitive)
	AnnotationDebug = "profiler.pprof.dev/debug"

	// AnnotationSidecarMetricsPort allows setting a custom metrics port for the sidecar
	// If not set, the sidecar defaults to 8080
	AnnotationSidecarMetricsPort = "profiler.pprof.dev/sidecar_metrics_port"
)
