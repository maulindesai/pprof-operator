package kubelet

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

var logger logr.Logger

// SetLogger allows main to inject a logger
func SetLogger(l logr.Logger) { logger = l }

// Config contains the dependencies required to query kubelet stats
type Config struct {
	Namespace       string
	PodName         string
	TargetContainer string
	K8sClient       *kubernetes.Clientset
}

// ContainerStats represents the container stats from the kubelet API
type ContainerStats struct {
	Name string `json:"name"`
	CPU  struct {
		UsageNanoCores uint64 `json:"usageNanoCores"`
	} `json:"cpu"`
	Memory struct {
		WorkingSetBytes uint64 `json:"workingSetBytes"`
	} `json:"memory"`
}

// PodStats represents the pod stats from the kubelet API
type PodStats struct {
	PodRef struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
		UID       string `json:"uid"`
	} `json:"podRef"`
	Containers []ContainerStats `json:"containers"`
}

// KubeletStats represents the stats from the kubelet API
type KubeletStats struct {
	Pods []PodStats `json:"pods"`
}

// GetKubeletMetrics reads CPU and memory metrics for the target container in percentage
func GetKubeletMetrics(cfg Config) (float64, float64, error) {
	// Check if we have the necessary information
	if cfg.PodName == "" || cfg.Namespace == "" || cfg.TargetContainer == "" {
		return 0, 0, fmt.Errorf("pod name, namespace, or target container not available")
	}

	// Get pod details to find the node name
	pod, err := cfg.K8sClient.CoreV1().Pods(cfg.Namespace).Get(context.Background(), cfg.PodName, metav1.GetOptions{})
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get pod details: %w", err)
	}

	nodeName := pod.Spec.NodeName
	if nodeName == "" {
		return 0, 0, fmt.Errorf("node name not available for pod %s", cfg.PodName)
	}

	logger.V(1).Info("Using node for metrics collection", "nodeName", nodeName)

	// The kubelet API endpoint for container stats is /api/v1/nodes/{nodeName}/proxy/stats/summary
	kubeletURL := fmt.Sprintf("/api/v1/nodes/%s/proxy/stats/summary", nodeName)
	logger.V(2).Info("Constructed kubelet API URL", "url", kubeletURL)

	// Use the Kubernetes client to proxy the request to the kubelet API
	result := cfg.K8sClient.CoreV1().RESTClient().Get().AbsPath(kubeletURL).Do(context.Background())
	rawData, err := result.Raw()
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get stats from kubelet API: %w", err)
	}

	// Parse the JSON response
	var stats KubeletStats
	if err := json.Unmarshal(rawData, &stats); err != nil {
		return 0, 0, fmt.Errorf("failed to parse kubelet API response: %w", err)
	}

	// Find the target pod and container in the stats
	var cpuUsage, memoryUsage uint64
	var cpuLimit, memoryLimit int64
	found := false

	// Find the target container in the pod spec to get resource limits
	for _, container := range pod.Spec.Containers {
		if container.Name == cfg.TargetContainer {
			// Get CPU limit
			if container.Resources.Limits != nil {
				if cpuLimitQuantity, ok := container.Resources.Limits["cpu"]; ok {
					cpuLimit = cpuLimitQuantity.MilliValue()
				}
			}
			// Get memory limit
			if container.Resources.Limits != nil {
				if memoryLimitQuantity, ok := container.Resources.Limits["memory"]; ok {
					memoryLimit = memoryLimitQuantity.Value()
				}
			}
			break
		}
	}

	// If no limits are set, get node capacity
	if cpuLimit == 0 || memoryLimit == 0 {
		logger.V(0).Info("No resource limits set for container, using node capacity",
			"container", cfg.TargetContainer,
			"namespace", cfg.Namespace,
			"pod", cfg.PodName)
		logger.V(0).Info("It's good practice to set CPU and memory limits for containers in pods")

		node, err := cfg.K8sClient.CoreV1().Nodes().Get(context.Background(), nodeName, metav1.GetOptions{})
		if err != nil {
			logger.Error(err, "Failed to get node details", "nodeName", nodeName)
			return 0, 0, fmt.Errorf("failed to get node details: %w", err)
		}

		if cpuLimit == 0 {
			if cpuCapacity, ok := node.Status.Capacity["cpu"]; ok {
				cpuLimit = cpuCapacity.MilliValue()
			}
		}

		if memoryLimit == 0 {
			if memoryCapacity, ok := node.Status.Capacity["memory"]; ok {
				memoryLimit = memoryCapacity.Value()
			}
		}
	}

	// Find the target pod and container in the stats
	for _, podStats := range stats.Pods {
		if podStats.PodRef.Namespace == cfg.Namespace && podStats.PodRef.Name == cfg.PodName {
			for _, containerStats := range podStats.Containers {
				if containerStats.Name == cfg.TargetContainer {
					cpuUsage = containerStats.CPU.UsageNanoCores
					memoryUsage = containerStats.Memory.WorkingSetBytes
					found = true
					break
				}
			}
			if found {
				break
			}
		}
	}

	if !found {
		return 0, 0, fmt.Errorf("container stats not found for %s in pod %s", cfg.TargetContainer, cfg.PodName)
	}

	// Calculate percentages
	cpuPercent := (float64(cpuUsage) / float64(cpuLimit*1000000)) * 100 // Convert milliCPU to nanoCPU
	memPercent := (float64(memoryUsage) / float64(memoryLimit)) * 100

	logger.Info("Resource usage metrics",
		"cpuPercent", fmt.Sprintf("%.2f%%", cpuPercent),
		"memPercent", fmt.Sprintf("%.2f%%", memPercent))

	logger.V(1).Info("Detailed resource metrics",
		"cpuUsage", cpuUsage,
		"cpuLimit", cpuLimit,
		"cpuPercent", cpuPercent,
		"memoryUsage", memoryUsage,
		"memoryLimit", memoryLimit,
		"memPercent", memPercent)

	return cpuPercent, memPercent, nil
}
