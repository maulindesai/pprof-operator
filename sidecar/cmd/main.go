package main

// This sidecar supports reading CPU and memory metrics from containers running under
// different container runtimes (Docker, containerd, CRI-O) using the kubelet API (metrics.k8s.io).
// It uses pod UID to find container metrics.
// It monitors resource usage and collects profiles when thresholds are exceeded.
// The sidecar also supports QoS-aware profiling, adjusting thresholds based on the pod's QoS class.
//
// Logging System:
// The sidecar uses structured logging with different log levels for better debugging and monitoring:
// - Debug (V(1), V(2)): Detailed information useful for debugging
// - Info: General operational information
// - Warning (V(0)): Potential issues that don't cause failures
// - Error: Actual errors that might require attention
// - Panic: Critical issues that require immediate attention
//
// To set the log level, use the LOG_LEVEL environment variable with one of:
// "debug", "info", "warn"/"warning", "error", or "panic"
// For backward compatibility, setting DEBUG=true will enable debug level logging.

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/go-logr/logr"
	"github.com/maulindesai/pprof-operator/sidecar/pkg/kubelet"
	"github.com/maulindesai/pprof-operator/sidecar/pkg/logging"
	"github.com/maulindesai/pprof-operator/sidecar/pkg/metrics"
	"github.com/maulindesai/pprof-operator/sidecar/pkg/storage"
	"github.com/maulindesai/pprof-operator/sidecar/pkg/types"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Global logger
var logger logr.Logger

const (
	defaultCPUThreshold     = 80
	defaultMemoryThreshold  = 80
	defaultProfileDuration  = 30
	defaultMonitoringPeriod = 15 * time.Second
)

type Config struct {
	TargetContainer  string
	CPUThreshold     int
	MemoryThreshold  int
	ProfileDuration  int
	MonitoringPeriod time.Duration
	S3Bucket         string
	S3Region         string
	S3PathPrefix     string
	S3Endpoint       string
	S3ForcePathStyle bool
	AWSAccessKeyID   string
	AWSSecretKey     string

	//Scrap Target
	ScrapTarget types.ScrapTarget

	// Kubernetes related fields
	Namespace string
	PodName   string
	// Kubernetes clients
	K8sClient *kubernetes.Clientset

	// Pod UID for finding container metrics
	PodUID string
}

func loadConfig() (*Config, error) {
	var err error

	targetContainer := os.Getenv("TARGET_CONTAINER")
	if targetContainer == "" {
		return nil, fmt.Errorf("TARGET_CONTAINER environment variable is required")
	}

	cpuThresholdStr := os.Getenv("CPU_THRESHOLD")
	cpuThreshold := defaultCPUThreshold
	if cpuThresholdStr != "" {
		cpuThreshold, err = strconv.Atoi(cpuThresholdStr)
		if err != nil {
			logger.V(0).Info("Invalid CPU_THRESHOLD value, using default",
				"value", cpuThresholdStr,
				"default", defaultCPUThreshold,
				"error", err)
			cpuThreshold = defaultCPUThreshold
		} else {
			logger.V(1).Info("Using CPU threshold from environment", "threshold", cpuThreshold)
		}
	} else {
		logger.V(1).Info("Using default CPU threshold", "threshold", cpuThreshold)
	}

	memoryThresholdStr := os.Getenv("MEMORY_THRESHOLD")
	memoryThreshold := defaultMemoryThreshold
	if memoryThresholdStr != "" {
		memoryThreshold, err = strconv.Atoi(memoryThresholdStr)
		if err != nil {
			logger.V(0).Info("Invalid MEMORY_THRESHOLD value, using default",
				"value", memoryThresholdStr,
				"default", defaultMemoryThreshold,
				"error", err)
			memoryThreshold = defaultMemoryThreshold
		} else {
			logger.V(1).Info("Using memory threshold from environment", "threshold", memoryThreshold)
		}
	} else {
		logger.V(1).Info("Using default memory threshold", "threshold", memoryThreshold)
	}

	profileDurationStr := os.Getenv("PROFILE_DURATION")
	profileDuration := defaultProfileDuration
	if profileDurationStr != "" {
		profileDuration, err = strconv.Atoi(profileDurationStr)
		if err != nil {
			logger.V(0).Info("Invalid PROFILE_DURATION value, using default",
				"value", profileDurationStr,
				"default", defaultProfileDuration,
				"error", err)
			profileDuration = defaultProfileDuration
		} else {
			logger.V(1).Info("Using profile duration from environment", "duration", profileDuration)
		}
	} else {
		logger.V(1).Info("Using default profile duration", "duration", profileDuration)
	}

	s3Bucket := os.Getenv("S3_BUCKET")
	if s3Bucket == "" {
		return nil, fmt.Errorf("S3_BUCKET environment variable is required")
	}

	s3Region := os.Getenv("S3_REGION")
	if s3Region == "" {
		s3Region = "us-west-2" // Default region
	}

	s3PathPrefix := os.Getenv("S3_PATH_PREFIX")

	// Optional S3-compatible endpoint (e.g., DigitalOcean Spaces)
	s3Endpoint := os.Getenv("S3_ENDPOINT")
	forcePathStyle := false
	if v := os.Getenv("S3_FORCE_PATH_STYLE"); v != "" {
		// accept "true"/"1" (case-insensitive)
		if v == "1" || v == "true" || v == "TRUE" || v == "True" {
			forcePathStyle = true
		}
	}

	awsAccessKeyID := os.Getenv("AWS_ACCESS_KEY_ID")
	awsSecretKey := os.Getenv("AWS_SECRET_ACCESS_KEY")

	// Get Kubernetes namespace from the environment or use default
	namespace := os.Getenv("POD_NAMESPACE")
	if namespace == "" {
		namespace = "default"
		// no need for now
		// Try to get namespace from the service account
		//data, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
		//if err == nil {
		//	namespace = strings.TrimSpace(string(data))
		//}
		//
		//if namespace == "" {
		//	namespace = "default"
		//}
	}

	// Get pod name from the environment
	podName := os.Getenv("POD_NAME")
	if podName == "" {
		// Try to get pod name from hostname
		podName, err = os.Hostname()
		if err != nil {
			logger.Info("Could not determine pod name", "error", err)
		}
	}

	// Initialize Kubernetes client (still needed for pod info)
	k8sConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get Kubernetes config: %w", err)
	}

	k8sClient, err := kubernetes.NewForConfig(k8sConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	// Read monitoring period from environment
	monitoringPeriodStr := os.Getenv("MONITORING_PERIOD")
	monitoringPeriod := defaultMonitoringPeriod
	if monitoringPeriodStr != "" {
		monitoringPeriodSec, err := strconv.Atoi(monitoringPeriodStr)
		if err != nil {
			logger.V(0).Info("Invalid MONITORING_PERIOD value, using default",
				"value", monitoringPeriodStr,
				"default", defaultMonitoringPeriod,
				"error", err)
		} else {
			monitoringPeriod = time.Duration(monitoringPeriodSec) * time.Second
			logger.V(1).Info("Using monitoring period from environment", "period", monitoringPeriod)
		}
	} else {
		logger.V(1).Info("Using default monitoring period", "period", monitoringPeriod)
	}

	// Create the config
	config := &Config{
		TargetContainer:  targetContainer,
		CPUThreshold:     cpuThreshold,
		MemoryThreshold:  memoryThreshold,
		ProfileDuration:  profileDuration,
		MonitoringPeriod: monitoringPeriod,
		S3Bucket:         s3Bucket,
		S3Region:         s3Region,
		S3PathPrefix:     s3PathPrefix,
		S3Endpoint:       s3Endpoint,
		S3ForcePathStyle: forcePathStyle,
		AWSAccessKeyID:   awsAccessKeyID,
		AWSSecretKey:     awsSecretKey,
		Namespace:        namespace,
		PodName:          podName,
		K8sClient:        k8sClient,
		ScrapTarget:      getScrapTarget(),
	}
	return config, nil
}

func getScrapTarget() types.ScrapTarget {
	scarpTargetAuthType := os.Getenv("AUTH_TYPE")
	scrapURL := os.Getenv("SCRAPE_URL")
	authUsername := os.Getenv("AUTH_USERNAME")
	authPassword := os.Getenv("AUTH_PASSWORD")

	var auth types.Auth
	if types.AuthType(scarpTargetAuthType) == types.AuthTypeBasic {
		auth = types.Auth{
			Type: types.AuthTypeBasic,
			BasicAuth: &types.BasicAuth{
				Username:  authUsername,
				Password:  authPassword,
				SecretRef: nil,
			},
		}
	}

	ScrapTarget := types.ScrapTarget{
		ScrapeURL: scrapURL,
		Auth:      &auth,
	}

	return ScrapTarget
}

func collectAndUploadProfile(ctx context.Context, config *Config, profileType string, reason string) (err error) {
	logger.Info("Starting profile collection", "type", profileType, "reason", reason)
	logger.V(1).Info("Profile collection details",
		"type", profileType,
		"container", config.TargetContainer,
		"pod", config.PodName,
		"namespace", config.Namespace)

	// Create a temporary directory for the profile
	tempDir, err := os.MkdirTemp("", "pprof-*")
	if err != nil {
		logger.Error(err, "Failed to create temporary directory")
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer func() {
		logger.V(2).Info("Cleaning up temporary directory", "dir", tempDir)
		if removeErr := os.RemoveAll(tempDir); removeErr != nil {
			logger.Error(removeErr, "Error removing temporary directory", "dir", tempDir)
			if err == nil {
				err = removeErr
			}
		}
	}()

	// Generate profile filename
	timestamp := time.Now().UTC().Format("20060102-150405")
	profileFilename := fmt.Sprintf("%s-%s-%s-%s.pprof", profileType, config.PodName, config.TargetContainer, timestamp)
	profilePath := filepath.Join(tempDir, profileFilename)

	logger.V(1).Info("Generated profile path", "filename", profileFilename, "path", profilePath)

	// Collect the profile using pprof
	logger.Info("Collecting profile to file", "type", profileType)

	// Check if we have a scrap URL
	if config.ScrapTarget.ScrapeURL != "" {
		logger.V(1).Info("Using scrap URL for profile collection", "url", config.ScrapTarget.ScrapeURL)

		// Construct the URL with profile type and duration
		url := fmt.Sprintf("%s/%s", config.ScrapTarget.ScrapeURL, profileType)
		logger.V(1).Info("Constructed profile URL", "url", url, "duration", config.ProfileDuration)

		// Create a new HTTP request
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			logger.Error(err, "Failed to create HTTP request", "url", url)
			return fmt.Errorf("failed to create HTTP request: %w", err)
		}

		// Add basic authentication if provided
		if config.ScrapTarget.Auth != nil && config.ScrapTarget.Auth.BasicAuth != nil {
			username := config.ScrapTarget.Auth.BasicAuth.Username
			password := config.ScrapTarget.Auth.BasicAuth.Password
			if username != "" && password != "" {
				req.SetBasicAuth(username, password)
				logger.V(1).Info("Added basic authentication to request")
			} else {
				logger.V(1).Info("No basic authentication credentials provided")
			}
		} else {
			logger.V(2).Info("No authentication configuration found")
		}

		// Create an HTTP client
		client := &http.Client{
			Timeout: time.Duration(config.ProfileDuration+10) * time.Second,
		}
		logger.V(2).Info("Created HTTP client", "timeout", client.Timeout)

		// Send the request
		logger.Info("Sending HTTP request to collect profile", "url", url)
		resp, err := client.Do(req)
		if err != nil {
			logger.Error(err, "Failed to send HTTP request", "url", url)
			return fmt.Errorf("failed to send HTTP request: %w", err)
		}
		defer func() {
			logger.V(2).Info("Closing response body")
			if closeErr := resp.Body.Close(); closeErr != nil {
				logger.Error(closeErr, "Error closing response body")
				if err == nil {
					err = closeErr
				}
			}
		}()

		// Check the response status
		if resp.StatusCode != http.StatusOK {
			logger.Error(nil, "Received non-OK response", "status", resp.Status, "url", url)
			return fmt.Errorf("received non-OK response: %s", resp.Status)
		}
		logger.V(1).Info("Received successful response", "status", resp.Status)

		// Create the profile file
		logger.V(1).Info("Creating profile file", "path", profilePath)
		file, err := os.Create(profilePath)
		if err != nil {
			logger.Error(err, "Failed to create profile file", "path", profilePath)
			return fmt.Errorf("failed to create profile file: %w", err)
		}
		defer func() {
			logger.V(2).Info("Closing profile file", "path", profilePath)
			if closeErr := file.Close(); closeErr != nil {
				logger.Error(closeErr, "Error closing profile file", "path", profilePath)
				if err == nil {
					err = closeErr
				}
			}
		}()

		// Copy the response body to the file
		logger.V(1).Info("Writing profile data to file", "path", profilePath)
		bytesWritten, err := io.Copy(file, resp.Body)
		if err != nil {
			logger.Error(err, "Failed to write profile data", "path", profilePath)
			return fmt.Errorf("failed to write profile data: %w", err)
		}
		logger.V(1).Info("Profile data written to file", "bytes", bytesWritten, "path", profilePath)

		// Increment profile generated metric
		metrics.ProfilesGenerated.With(prometheus.Labels{"profile_type": profileType}).Inc()
		logger.Info("Successfully collected profile", "type", profileType, "url", url)
	} else {
		logger.V(0).Info("No scraping done - no scrap URL found")
		return fmt.Errorf("no scrap url found")
	}

	// Upload the profile to S3
	s3Key := profileFilename
	if config.S3PathPrefix != "" {
		s3Key = filepath.Join(config.S3PathPrefix, profileFilename)
		logger.V(1).Info("Using S3 path prefix", "prefix", config.S3PathPrefix, "key", s3Key)
	} else {
		logger.V(1).Info("No S3 path prefix specified, using filename as key", "key", s3Key)
	}

	logger.Info("Uploading profile to S3", "bucket", config.S3Bucket, "key", s3Key)
	s3cfg := storage.S3Config{
		S3Bucket:         config.S3Bucket,
		S3Region:         config.S3Region,
		S3PathPrefix:     config.S3PathPrefix,
		S3Endpoint:       config.S3Endpoint,
		S3ForcePathStyle: config.S3ForcePathStyle,
		AWSAccessKeyID:   config.AWSAccessKeyID,
		AWSSecretKey:     config.AWSSecretKey,
	}
	if uploadErr := storage.UploadToS3(ctx, s3cfg, profilePath, s3Key); uploadErr != nil {
		logger.Error(uploadErr, "Failed to upload profile to S3",
			"bucket", config.S3Bucket,
			"key", s3Key,
			"path", profilePath)
		return fmt.Errorf("failed to upload profile to S3: %w", uploadErr)
	}

	logger.Info("Successfully collected and uploaded profile",
		"type", profileType,
		"bucket", config.S3Bucket,
		"key", s3Key)
	return nil
}

func main() {
	// Initialize structured logger via shared logging package
	l, cleanup := logging.InitFromEnv()
	defer cleanup()

	// Set global logger and propagate to internal packages
	logger = l
	storage.SetLogger(logger)
	kubelet.SetLogger(logger)

	logger.Info("Starting pprof sidecar with kubelet API metrics (metrics.k8s.io) and QoS support",
		"runtimes", "Docker, containerd, CRI-O")

	// Load configuration from environment variables
	config, err := loadConfig()
	if err != nil {
		logger.Error(err, "Failed to load configuration")
		os.Exit(1)
	}

	logger.Info("Configuration loaded",
		"target", config.TargetContainer,
		"namespace", config.Namespace,
		"pod", config.PodName,
		"podUID", config.PodUID,
		"cpuThreshold", config.CPUThreshold,
		"memoryThreshold", config.MemoryThreshold,
		"profileDuration", config.ProfileDuration,
		"monitoringPeriod", config.MonitoringPeriod)

	logger.V(1).Info("Detailed configuration",
		"s3Bucket", config.S3Bucket,
		"s3Region", config.S3Region,
		"s3PathPrefix", config.S3PathPrefix,
		"scrapTarget", config.ScrapTarget)

	// Create context that can be cancelled
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle termination signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		logger.Info("Received termination signal, shutting down", "signal", sig)
		cancel()
	}()

	// Start a metrics server on port 8080
	metricsAddr := os.Getenv("METRICS_ADDR")
	if metricsAddr == "" {
		metricsAddr = ":8080"
	}

	logger.Info("Starting metrics server", "address", metricsAddr)
	go func() {
		if err := metrics.StartMetricsServer(metricsAddr); err != nil {
			logger.Error(err, "Failed to start metrics server")
		}
	}()

	// Start monitoring
	monitorResourceUsage(ctx, config)
}

// monitorResourceUsage periodically fetches usage and triggers profiling when thresholds exceeded
func monitorResourceUsage(ctx context.Context, config *Config) {
	logger.Info("Starting resource usage monitoring",
		"period", config.MonitoringPeriod,
		"cpuThreshold", config.CPUThreshold,
		"memThreshold", config.MemoryThreshold)

	ticker := time.NewTicker(config.MonitoringPeriod)
	defer ticker.Stop()

	var lastCPUTime time.Time

	cpuThreshold := config.CPUThreshold
	memThreshold := config.MemoryThreshold

	logger.V(1).Info("Using thresholds for profiling",
		"cpuThreshold", cpuThreshold,
		"memThreshold", memThreshold)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Get container metrics using kubelet API
			cpuPercent, memoryPercent, err := kubelet.GetKubeletMetrics(kubelet.Config{
				Namespace:       config.Namespace,
				PodName:         config.PodName,
				TargetContainer: config.TargetContainer,
				K8sClient:       config.K8sClient,
			})
			if err != nil {
				logger.Error(err, "Error getting container metrics")
				continue
			}

			// Update resource usage metrics
			metrics.ResourceUsage.With(prometheus.Labels{"resource_type": "cpu"}).Set(cpuPercent)
			metrics.ResourceUsage.With(prometheus.Labels{"resource_type": "memory"}).Set(memoryPercent)

			// Check thresholds
			cpuThresholdExceeded := cpuPercent >= float64(cpuThreshold)
			memThresholdExceeded := memoryPercent >= float64(memThreshold)

			logger.V(2).Info("Threshold status",
				"cpuExceeded", cpuThresholdExceeded,
				"memExceeded", memThresholdExceeded)

			if cpuThresholdExceeded {
				metrics.ThresholdExceeded.With(prometheus.Labels{"resource_type": "cpu"}).Inc()
			}
			if memThresholdExceeded {
				metrics.ThresholdExceeded.With(prometheus.Labels{"resource_type": "memory"}).Inc()
			}

			now := time.Now()
			if lastCPUTime.IsZero() || now.Sub(lastCPUTime) > time.Minute*5 {
				if cpuThresholdExceeded {
					logger.V(0).Info("CPU threshold exceeded, collecting CPU profile",
						"cpuUsage", fmt.Sprintf("%.2f%%", cpuPercent),
						"threshold", cpuThreshold)
					if err := collectAndUploadProfile(ctx, config, "cpu",
						fmt.Sprintf("CPU threshold exceeded: %.2f%%", cpuPercent)); err != nil {
						logger.Error(err, "Failed to collect CPU profile")
					} else {
						logger.Info("Successfully collected CPU profile")
					}
					lastCPUTime = now
				} else {
					logger.V(2).Info("CPU threshold not exceeded or cooldown period active",
						"cpuThresholdExceeded", cpuThresholdExceeded,
						"cpuUsage", fmt.Sprintf("%.2f%%", cpuPercent),
						"threshold", cpuThreshold,
						"timeSinceLastProfile", now.Sub(lastCPUTime))
				}
			}

			if memThresholdExceeded {
				logger.V(0).Info("Memory threshold exceeded, collecting heap profile",
					"memoryUsage", fmt.Sprintf("%.2f%%", memoryPercent),
					"threshold", memThreshold)
				if err := collectAndUploadProfile(ctx, config, "heap",
					fmt.Sprintf("Memory threshold exceeded: %.2f%%", memoryPercent)); err != nil {
					logger.Error(err, "Failed to collect heap profile")
				} else {
					logger.Info("Successfully collected heap profile")
				}
			} else {
				logger.V(2).Info("Memory threshold not exceeded",
					"memoryUsage", fmt.Sprintf("%.2f%%", memoryPercent),
					"threshold", memThreshold)
			}
		}
	}
}
