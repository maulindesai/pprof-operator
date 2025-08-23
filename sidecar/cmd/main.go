package main

// This sidecar supports reading CPU and memory metrics from containers running under
// different container runtimes (Docker, containerd, CRI-O) and with either cgroup v1 or cgroup v2.
// It monitors resource usage and collects profiles when thresholds are exceeded.

import (
	"bufio"
	"context"
	"fmt"
	v1 "github.com/maulindesai/pprof-operator/api/v1"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	sdkconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

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
	AWSAccessKeyID   string
	AWSSecretKey     string

	//Scrap Target
	ScrapTarget v1.ScrapTarget

	// Kubernetes related fields
	Namespace string
	PodName   string
	// Kubernetes clients
	K8sClient *kubernetes.Clientset

	// Container ID for cgroup metrics
	ContainerID string
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
			log.Printf("Warning: Invalid CPU_THRESHOLD value '%s', using default %d", cpuThresholdStr, defaultCPUThreshold)
			cpuThreshold = defaultCPUThreshold
		}
	}

	memoryThresholdStr := os.Getenv("MEMORY_THRESHOLD")
	memoryThreshold := defaultMemoryThreshold
	if memoryThresholdStr != "" {
		memoryThreshold, err = strconv.Atoi(memoryThresholdStr)
		if err != nil {
			log.Printf("Warning: Invalid MEMORY_THRESHOLD value '%s', using default %d", memoryThresholdStr, defaultMemoryThreshold)
			memoryThreshold = defaultMemoryThreshold
		}
	}

	profileDurationStr := os.Getenv("PROFILE_DURATION")
	profileDuration := defaultProfileDuration
	if profileDurationStr != "" {
		profileDuration, err = strconv.Atoi(profileDurationStr)
		if err != nil {
			log.Printf("Warning: Invalid PROFILE_DURATION value '%s', using default %d", profileDurationStr, defaultProfileDuration)
			profileDuration = defaultProfileDuration
		}
	}

	s3Bucket := os.Getenv("S3_BUCKET")
	if s3Bucket == "" {
		return nil, fmt.Errorf("S3_BUCKET environment variable is required")
	}

	s3Region := os.Getenv("S3_REGION")
	if s3Region == "" {
		s3Region = "us-east-1" // Default region
	}

	s3PathPrefix := os.Getenv("S3_PATH_PREFIX")

	awsAccessKeyID := os.Getenv("AWS_ACCESS_KEY_ID")
	awsSecretKey := os.Getenv("AWS_SECRET_ACCESS_KEY")

	// Get Kubernetes namespace from the environment or use default
	namespace := os.Getenv("POD_NAMESPACE")
	if namespace == "" {
		// Try to get namespace from the service account
		data, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
		if err == nil {
			namespace = strings.TrimSpace(string(data))
		}

		if namespace == "" {
			namespace = "default"
		}
	}

	// Get pod name from the environment
	podName := os.Getenv("POD_NAME")
	if podName == "" {
		// Try to get pod name from hostname
		podName, err = os.Hostname()
		if err != nil {
			log.Printf("Warning: Could not determine pod name: %v", err)
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
			log.Printf("Warning: Invalid MONITORING_PERIOD value '%s', using default %s", monitoringPeriodStr, defaultMonitoringPeriod)
		} else {
			monitoringPeriod = time.Duration(monitoringPeriodSec) * time.Second
			log.Printf("Using monitoring period from environment: %s", monitoringPeriod)
		}
	}

	// Create the config
	config := &Config{
		TargetContainer:  targetContainer,
		ContainerID:      "", // Will be populated later
		CPUThreshold:     cpuThreshold,
		MemoryThreshold:  memoryThreshold,
		ProfileDuration:  profileDuration,
		MonitoringPeriod: monitoringPeriod,
		S3Bucket:         s3Bucket,
		S3Region:         s3Region,
		S3PathPrefix:     s3PathPrefix,
		AWSAccessKeyID:   awsAccessKeyID,
		AWSSecretKey:     awsSecretKey,
		Namespace:        namespace,
		PodName:          podName,
		K8sClient:        k8sClient,
		ScrapTarget:      getScrapTarget(),
	}

	// Find the container ID for the target container
	containerID, err := findContainerID(targetContainer, k8sClient, namespace, podName)
	if err != nil {
		log.Printf("Warning: Failed to find container ID: %v", err)
	} else {
		config.ContainerID = containerID
		log.Printf("Found container ID for %s: %s", targetContainer, containerID)
	}

	return config, nil
}

func getScrapTarget() v1.ScrapTarget {
	scarpTargetAuthType := os.Getenv("AUTH_TYPE")
	scrapURL := os.Getenv("SCRAP_URL")
	authUsername := os.Getenv("AUTH_USERNAME")
	authPassword := os.Getenv("AUTH_PASSWORD")

	var auth v1.Auth
	if v1.AuthType(scarpTargetAuthType) == v1.AuthTypeBasic {
		auth = v1.Auth{
			Type: v1.AuthTypeBasic,
			BasicAuth: &v1.BasicAuth{
				Username:  authUsername,
				Password:  authPassword,
				SecretRef: nil,
			},
		}
	}

	ScrapTarget := v1.ScrapTarget{
		ScrapeURL: scrapURL,
		Auth:      &auth,
	}

	return ScrapTarget
}

// findContainerID finds the container ID for the target container using Kubernetes API
func findContainerID(targetContainer string, k8sClient *kubernetes.Clientset, namespace string, podName string) (string, error) {
	// Get pod details from Kubernetes API
	pod, err := k8sClient.CoreV1().Pods(namespace).Get(context.Background(), podName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get pod details from Kubernetes API: %w", err)
	}

	// Find the target container in the pod's status
	for _, containerStatus := range pod.Status.ContainerStatuses {
		if containerStatus.Name == targetContainer {
			// Extract container ID from the containerID string (format: docker://containerID or containerd://containerID)
			containerIDParts := strings.Split(containerStatus.ContainerID, "://")
			if len(containerIDParts) == 2 {
				return containerIDParts[1], nil
			}
			return containerStatus.ContainerID, nil
		}
	}

	// If container not found in main containers, check init containers
	for _, containerStatus := range pod.Status.InitContainerStatuses {
		if containerStatus.Name == targetContainer {
			containerIDParts := strings.Split(containerStatus.ContainerID, "://")
			if len(containerIDParts) == 2 {
				return containerIDParts[1], nil
			}
			return containerStatus.ContainerID, nil
		}
	}
	return "", fmt.Errorf("container ID not found for %s in pod %s", targetContainer, podName)
}

// getCgroupMetrics reads CPU and memory metrics from cgroup files
func getCgroupMetrics(config *Config) (float64, float64, error) {
	if config.ContainerID == "" {
		return 0, 0, fmt.Errorf("container ID not available")
	}

	// Get CPU usage from cgroup
	cpuUsage, cpuLimit, err := getCPUMetrics(config.ContainerID)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get CPU metrics: %w", err)
	}

	// Get memory usage from cgroup
	memUsage, memLimit, err := getMemoryMetrics(config.ContainerID)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get memory metrics: %w", err)
	}

	// Calculate percentages
	cpuPercent := (float64(cpuUsage) / float64(cpuLimit)) * 100
	memPercent := (float64(memUsage) / float64(memLimit)) * 100

	return cpuPercent, memPercent, nil
}

// getCPUMetrics reads CPU usage and limit from cgroup files
func getCPUMetrics(containerID string) (uint64, uint64, error) {
	// Try different possible cgroup paths for various container runtimes (docker, containerd, crio)
	cgroupPaths := []string{
		// Docker paths
		fmt.Sprintf("/sys/fs/cgroup/cpu/docker/%s", containerID),
		fmt.Sprintf("/sys/fs/cgroup/cpu/kubepods/*/docker-%s.scope", containerID),
		fmt.Sprintf("/sys/fs/cgroup/cpu/kubepods/*/*/docker-%s.scope", containerID),
		fmt.Sprintf("/sys/fs/cgroup/cpu/kubepods.slice/kubepods-*.slice/docker-%s.scope", containerID),

		// Containerd paths
		fmt.Sprintf("/sys/fs/cgroup/cpu/kubepods/*/containerd-%s.scope", containerID),
		fmt.Sprintf("/sys/fs/cgroup/cpu/kubepods/*/*/containerd-%s.scope", containerID),
		fmt.Sprintf("/sys/fs/cgroup/cpu/kubepods.slice/kubepods-*.slice/containerd-%s.scope", containerID),

		// CRI-O paths
		fmt.Sprintf("/sys/fs/cgroup/cpu/kubepods/*/crio-%s.scope", containerID),
		fmt.Sprintf("/sys/fs/cgroup/cpu/kubepods/*/*/crio-%s.scope", containerID),
		fmt.Sprintf("/sys/fs/cgroup/cpu/kubepods.slice/kubepods-*.slice/crio-%s.scope", containerID),

		// Unified cgroup v2 paths (for all runtimes)
		fmt.Sprintf("/sys/fs/cgroup/kubepods.slice/kubepods-*.slice/*/%s", containerID),
	}

	var cpuUsage, cpuLimit uint64

	for _, pathPattern := range cgroupPaths {
		matches, _ := filepath.Glob(pathPattern)
		if len(matches) > 0 {
			// Check if this is cgroup v2 by looking for cpu.stat file
			_, err := os.Stat(filepath.Join(matches[0], "cpu.stat"))
			isCgroupV2 := err == nil

			if isCgroupV2 {
				// Read CPU usage from cgroup v2
				statData, err := os.ReadFile(filepath.Join(matches[0], "cpu.stat"))
				if err == nil {
					// Parse the cpu.stat file to extract usage_usec
					scanner := bufio.NewScanner(strings.NewReader(string(statData)))
					for scanner.Scan() {
						line := scanner.Text()
						if strings.HasPrefix(line, "usage_usec") {
							fields := strings.Fields(line)
							if len(fields) >= 2 {
								// Convert microseconds to nanoseconds for consistency with cgroup v1
								usageMicros, _ := strconv.ParseUint(fields[1], 10, 64)
								cpuUsage = usageMicros * 1000
								break
							}
						}
					}
				}

				// Read CPU limit from cgroup v2
				maxData, err := os.ReadFile(filepath.Join(matches[0], "cpu.max"))
				if err == nil {
					fields := strings.Fields(string(maxData))
					if len(fields) >= 2 && fields[0] != "max" {
						quota, _ := strconv.ParseInt(fields[0], 10, 64)
						period, _ := strconv.ParseUint(fields[1], 10, 64)
						if quota > 0 && period > 0 {
							cpuLimit = uint64(quota) * 100 / period
						}
					}
				}
			} else {
				// Read CPU usage from cgroup v1
				usageData, err := os.ReadFile(filepath.Join(matches[0], "cpuacct.usage"))
				if err == nil {
					cpuUsage, _ = strconv.ParseUint(strings.TrimSpace(string(usageData)), 10, 64)
				}

				// Read CPU limit (quota and period) from cgroup v1
				quotaData, err := os.ReadFile(filepath.Join(matches[0], "cpu.cfs_quota_us"))
				if err == nil {
					quota, _ := strconv.ParseInt(strings.TrimSpace(string(quotaData)), 10, 64)
					if quota > 0 {
						periodData, err := os.ReadFile(filepath.Join(matches[0], "cpu.cfs_period_us"))
						if err == nil {
							period, _ := strconv.ParseUint(strings.TrimSpace(string(periodData)), 10, 64)
							if period > 0 {
								cpuLimit = uint64(quota) * 100 / period
							}
						}
					}
				}
			}

			// If we couldn't get the limit, use a default value
			if cpuLimit == 0 {
				cpuLimit = 100 * 100000 // Assume 1 CPU = 100%
			}

			return cpuUsage, cpuLimit, nil
		}
	}

	return 0, 0, fmt.Errorf("failed to find cgroup CPU metrics for container %s", containerID)
}

// getMemoryMetrics reads memory usage and limit from cgroup files
func getMemoryMetrics(containerID string) (uint64, uint64, error) {
	// Try different possible cgroup paths for various container runtimes (docker, containerd, crio)
	cgroupPaths := []string{
		// Docker paths
		fmt.Sprintf("/sys/fs/cgroup/memory/docker/%s", containerID),
		fmt.Sprintf("/sys/fs/cgroup/memory/kubepods/*/docker-%s.scope", containerID),
		fmt.Sprintf("/sys/fs/cgroup/memory/kubepods/*/*/docker-%s.scope", containerID),
		fmt.Sprintf("/sys/fs/cgroup/memory/kubepods.slice/kubepods-*.slice/docker-%s.scope", containerID),

		// Containerd paths
		fmt.Sprintf("/sys/fs/cgroup/memory/kubepods/*/containerd-%s.scope", containerID),
		fmt.Sprintf("/sys/fs/cgroup/memory/kubepods/*/*/containerd-%s.scope", containerID),
		fmt.Sprintf("/sys/fs/cgroup/memory/kubepods.slice/kubepods-*.slice/containerd-%s.scope", containerID),

		// CRI-O paths
		fmt.Sprintf("/sys/fs/cgroup/memory/kubepods/*/crio-%s.scope", containerID),
		fmt.Sprintf("/sys/fs/cgroup/memory/kubepods/*/*/crio-%s.scope", containerID),
		fmt.Sprintf("/sys/fs/cgroup/memory/kubepods.slice/kubepods-*.slice/crio-%s.scope", containerID),

		// Unified cgroup v2 paths (for all runtimes)
		fmt.Sprintf("/sys/fs/cgroup/kubepods.slice/kubepods-*.slice/*/%s", containerID),
	}

	var memUsage, memLimit uint64

	for _, pathPattern := range cgroupPaths {
		matches, _ := filepath.Glob(pathPattern)
		if len(matches) > 0 {
			// Check if this is cgroup v2 by looking for memory.stat file
			_, err := os.Stat(filepath.Join(matches[0], "memory.stat"))
			isCgroupV2 := err == nil

			if isCgroupV2 {
				// Read memory usage from cgroup v2
				usageData, err := os.ReadFile(filepath.Join(matches[0], "memory.current"))
				if err == nil {
					memUsage, _ = strconv.ParseUint(strings.TrimSpace(string(usageData)), 10, 64)
				}

				// Read memory limit from cgroup v2
				limitData, err := os.ReadFile(filepath.Join(matches[0], "memory.max"))
				if err == nil {
					limitStr := strings.TrimSpace(string(limitData))
					if limitStr != "max" { // "max" means no limit
						memLimit, _ = strconv.ParseUint(limitStr, 10, 64)
					}
				}
			} else {
				// Read memory usage from cgroup v1
				usageData, err := os.ReadFile(filepath.Join(matches[0], "memory.usage_in_bytes"))
				if err == nil {
					memUsage, _ = strconv.ParseUint(strings.TrimSpace(string(usageData)), 10, 64)
				}

				// Read memory limit from cgroup v1
				limitData, err := os.ReadFile(filepath.Join(matches[0], "memory.limit_in_bytes"))
				if err == nil {
					memLimit, _ = strconv.ParseUint(strings.TrimSpace(string(limitData)), 10, 64)
				}
			}

			// If the limit is too high (e.g., 9223372036854771712), it's effectively unlimited
			// In that case, use the host's total memory as a reference
			if memLimit > 1<<42 { // 4TB, an arbitrary high value
				totalMemData, err := os.ReadFile("/proc/meminfo")
				if err == nil {
					scanner := bufio.NewScanner(strings.NewReader(string(totalMemData)))
					for scanner.Scan() {
						line := scanner.Text()
						if strings.HasPrefix(line, "MemTotal:") {
							fields := strings.Fields(line)
							if len(fields) >= 2 {
								memTotal, _ := strconv.ParseUint(fields[1], 10, 64)
								memLimit = memTotal * 1024 // Convert from KB to bytes
								break
							}
						}
					}
				}
			}

			// If we still don't have a valid limit, use a default
			if memLimit == 0 {
				memLimit = 8 * 1024 * 1024 * 1024 // 8GB default
			}

			return memUsage, memLimit, nil
		}
	}

	return 0, 0, fmt.Errorf("failed to find cgroup memory metrics for container %s", containerID)
}

func monitorResourceUsage(ctx context.Context, config *Config) {
	ticker := time.NewTicker(config.MonitoringPeriod)
	defer ticker.Stop()

	var lastCPUTime time.Time

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Get container metrics using cgroup
			cpuPercent, memoryPercent, err := getCgroupMetrics(config)
			if err != nil {
				log.Printf("Error getting container metrics: %v", err)

				// If container ID is not available or has changed, try to find it again
				if config.ContainerID == "" || strings.Contains(err.Error(), "failed to find cgroup") {
					containerID, err := findContainerID(config.TargetContainer, config.K8sClient, config.Namespace, config.PodName)
					if err != nil {
						log.Printf("Failed to find container ID: %v", err)
					} else {
						config.ContainerID = containerID
						log.Printf("Updated container ID for %s: %s", config.TargetContainer, containerID)
					}
				}
				continue
			}

			log.Printf("Resource usage - CPU: %.2f%%, Memory: %.2f%%", cpuPercent, memoryPercent)

			// Check if thresholds are exceeded
			cpuThresholdExceeded := cpuPercent >= float64(config.CPUThreshold)
			memThresholdExceeded := memoryPercent >= float64(config.MemoryThreshold)

			// Avoid collecting profiles too frequently
			now := time.Now()
			if lastCPUTime.IsZero() || now.Sub(lastCPUTime) > time.Minute*5 {
				if cpuThresholdExceeded {
					log.Printf("CPU threshold exceeded (%.2f%% >= %d%%), collecting CPU profile", cpuPercent, config.CPUThreshold)
					if err := collectAndUploadProfile(ctx, config, "cpu", fmt.Sprintf("CPU threshold exceeded: %.2f%%", cpuPercent)); err != nil {
						log.Printf("Failed to collect CPU profile: %v", err)
					}
					lastCPUTime = now
				}
			}

			// For memory, we'll use a similar approach
			if memThresholdExceeded {
				log.Printf("Memory threshold exceeded (%.2f%% >= %d%%), collecting heap profile", memoryPercent, config.MemoryThreshold)
				if err := collectAndUploadProfile(ctx, config, "heap", fmt.Sprintf("Memory threshold exceeded: %.2f%%", memoryPercent)); err != nil {
					log.Printf("Failed to collect heap profile: %v", err)
				}
			}
		}
	}
}

func collectAndUploadProfile(ctx context.Context, config *Config, profileType string, reason string) (err error) {
	log.Printf("Collecting %s profile for %s", profileType, reason)
	// Create a temporary directory for the profile
	tempDir, err := os.MkdirTemp("", "pprof-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer func() {
		if removeErr := os.RemoveAll(tempDir); removeErr != nil {
			log.Printf("Error removing temporary directory %s: %v", tempDir, removeErr)
			if err == nil {
				err = removeErr
			}
		}
	}()

	// Generate profile filename
	timestamp := time.Now().UTC().Format("20060102-150405")
	profileFilename := fmt.Sprintf("%s-%s-%s-%s.pprof", profileType, config.PodName, config.TargetContainer, timestamp)
	profilePath := filepath.Join(tempDir, profileFilename)

	// Collect the profile using pprof
	// In a real implementation, we would use the pprof HTTP endpoint of the target process
	log.Printf("Collecting %s profile to %s", profileType, profilePath)

	// Check if we have a scrap URL
	if config.ScrapTarget.ScrapeURL != "" {
		log.Printf("Using scrap URL: %s", config.ScrapTarget.ScrapeURL)

		// Construct the URL with profile type and duration
		url := fmt.Sprintf("%s/%s?seconds=%d", config.ScrapTarget.ScrapeURL, profileType, config.ProfileDuration)

		// Create a new HTTP request
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return fmt.Errorf("failed to create HTTP request: %w", err)
		}

		// Add basic authentication if provided
		if config.ScrapTarget.Auth != nil && config.ScrapTarget.Auth.BasicAuth != nil {
			username := config.ScrapTarget.Auth.BasicAuth.Username
			password := config.ScrapTarget.Auth.BasicAuth.Password
			if username != "" && password != "" {
				req.SetBasicAuth(username, password)
			}
		}

		// Create an HTTP client
		client := &http.Client{
			Timeout: time.Duration(config.ProfileDuration+10) * time.Second,
		}

		// Send the request
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to send HTTP request: %w", err)
		}
		defer func() {
			if closeErr := resp.Body.Close(); closeErr != nil {
				log.Printf("Error closing response body: %v", closeErr)
				if err == nil {
					err = closeErr
				}
			}
		}()

		// Check the response status
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("received non-OK response: %s", resp.Status)
		}

		// Create the profile file
		file, err := os.Create(profilePath)
		if err != nil {
			return fmt.Errorf("failed to create profile file: %w", err)
		}
		defer func() {
			if closeErr := file.Close(); closeErr != nil {
				log.Printf("Error closing profile file %s: %v", profilePath, closeErr)
				if err == nil {
					err = closeErr
				}
			}
		}()

		// Copy the response body to the file
		_, err = io.Copy(file, resp.Body)
		if err != nil {
			return fmt.Errorf("failed to write profile data: %w", err)
		}

		log.Printf("Successfully collected profile from %s", url)
	} else {
		log.Printf("No Scrapping done. No scrap url found.")
		return fmt.Errorf("no scrap url found")
	}

	// Upload the profile to S3
	s3Key := profileFilename
	if config.S3PathPrefix != "" {
		s3Key = filepath.Join(config.S3PathPrefix, profileFilename)
	}

	if uploadErr := uploadToS3(ctx, config, profilePath, s3Key); uploadErr != nil {
		return fmt.Errorf("failed to upload profile to S3: %w", uploadErr)
	}

	log.Printf("Successfully collected and uploaded %s profile to s3://%s/%s", profileType, config.S3Bucket, s3Key)
	return nil
}

func uploadToS3(ctx context.Context, config *Config, filePath, s3Key string) error {
	// Create AWS config
	var awsConfig aws.Config
	var err error

	if config.AWSAccessKeyID != "" && config.AWSSecretKey != "" {
		// Use provided credentials
		awsConfig, err = config.LoadWithCredentials(ctx)
	} else {
		// Use default credentials provider chain
		awsConfig, err = config.LoadDefaultConfig(ctx)
	}

	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create S3 client
	s3Client := s3.NewFromConfig(awsConfig)

	// Open the file
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			log.Printf("Error closing file %s: %v", filePath, closeErr)
			if err == nil {
				err = closeErr
			}
		}
	}()

	// Upload the file
	_, err = s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(config.S3Bucket),
		Key:    aws.String(s3Key),
		Body:   file,
	})
	if err != nil {
		return fmt.Errorf("failed to upload file to S3: %w", err)
	}

	return nil
}

func (c *Config) LoadWithCredentials(ctx context.Context) (aws.Config, error) {
	return sdkconfig.LoadDefaultConfig(ctx,
		sdkconfig.WithRegion(c.S3Region),
		sdkconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			c.AWSAccessKeyID,
			c.AWSSecretKey,
			"",
		)),
	)
}

func (c *Config) LoadDefaultConfig(ctx context.Context) (aws.Config, error) {
	return sdkconfig.LoadDefaultConfig(ctx,
		sdkconfig.WithRegion(c.S3Region),
	)
}

func main() {
	log.Println("Starting pprof sidecar with cgroup metrics (supports Docker, containerd, CRI-O, and both cgroup v1/v2)")

	// Load configuration from environment variables
	config, err := loadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	log.Printf("Configuration loaded: target=%s, namespace=%s, pod=%s, CPU threshold=%d%%, Memory threshold=%d%%, Profile duration=%ds, Monitoring period=%s",
		config.TargetContainer, config.Namespace, config.PodName, config.CPUThreshold, config.MemoryThreshold, config.ProfileDuration, config.MonitoringPeriod)

	// Create context that can be cancelled
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle termination signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Printf("Received signal %v, shutting down", sig)
		cancel()
	}()

	// Start monitoring
	monitorResourceUsage(ctx, config)
}
