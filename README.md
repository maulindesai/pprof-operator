# pprof-operator

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![Go Report Card](https://goreportcard.com/badge/github.com/maulindesai/pprof-operator)](https://goreportcard.com/report/github.com/maulindesai/pprof-operator)
[![Go Version](https://img.shields.io/github/go-mod/go-version/maulindesai/pprof-operator)](https://github.com/maulindesai/pprof-operator)
[![Kubernetes](https://img.shields.io/badge/kubernetes-%23326ce5.svg?style=flat&logo=kubernetes&logoColor=white)](https://kubernetes.io/)
[![Docker](https://img.shields.io/badge/docker-%230db7ed.svg?style=flat&logo=docker&logoColor=white)](https://www.docker.com/)

A Kubernetes operator for automatically collecting pprof profiles from Go applications when CPU or memory thresholds are exceeded, helping you diagnose and resolve performance issues in production environments.

<div align="center">
  <img src="https://github.com/maulindesai/pprof-operator/blob/main/brand/logo.svg" alt="Go Gopher" width="300"/>
</div>

## 📑 Table of Contents

- [Overview](#-overview)
- [Architecture](#-architecture)
- [Metrics](#-metrics)
- [Security Considerations](#-security-considerations)
  - [Performance Impact Considerations](#performance-impact-considerations)
- [Quick Start](#-quick-start)
- [Getting Started](#-getting-started)
  - [Prerequisites](#prerequisites)
  - [Installation](#installation)
  - [Configuration](#configuration)
    - [Profiler CRD Configuration Reference](#profiler-crd-configuration-reference)
  - [Uninstallation](#uninstallation)
- [Sample Application](#-sample-application)
  - [Viewing and Analyzing Profiles](#viewing-and-analyzing-profiles)
  - [Profile Analysis Guide](#profile-analysis-guide)
  - [Common Performance Issues to Look For](#common-performance-issues-to-look-for)
  - [Advanced Analysis Techniques](#advanced-analysis-techniques)
- [Distribution](#-distribution)
- [Troubleshooting](#-troubleshooting)
- [Contributing](#-contributing)
- [Additional Resources](#-additional-resources)
  - [Related Projects](#related-projects)
  - [Further Reading](#further-reading)
- [License](#license)

## 📋 Overview

The pprof-operator is designed to help developers and operators diagnose performance issues in their Go applications running on Kubernetes. It works by:

1. 🔍 Monitoring containers for CPU and memory usage using the Kubernetes metrics API
2. 📊 Automatically collecting pprof profiles when thresholds are exceeded
3. 🚀 Uploading the profiles to AWS S3 for later analysis
4. 📈 Tracking profiling history in the Profiler resource status

**Key Features:**
- Threshold-based profiling (CPU and memory)
- Automatic sidecar injection via webhooks
- Support for multiple container runtimes (Docker, containerd, CRI-O)
- Authentication support for pprof endpoints
- Configurable monitoring periods and profile durations
- Structured logging with different verbosity levels
- Comprehensive metrics for monitoring operator performance

The operator consists of two main components:
- **Controller**: Manages the Profiler CRD and injects the sidecar container into target pods
- **Sidecar**: Monitors the target container and collects profiles when thresholds are exceeded

## 🏗️ Architecture

The pprof-operator uses a Kubernetes operator pattern with the following components:

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│                 │     │                 │     │                 │
│  Profiler CRD   │◄────┤   Controller    │────►│    Webhook      │
│                 │     │                 │     │                 │
└─────────────────┘     └─────────────────┘     └────────┬────────┘
                                                         │
                                                         ▼
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│                 │     │                 │     │                 │
│  Target Pod     │     │  Sidecar        │────►│   AWS S3        │
│                 │     │  Container      │     │                 │
└─────────────────┘     └─────────────────┘     └─────────────────┘
```

1. **Profiler Custom Resource**: Defines which applications to monitor and the thresholds for profiling
2. **Controller**: Watches for Profiler resources and manages the lifecycle of profiling sidecars
3. **Webhook**: Injects profiling sidecars into pods that match the selector in the Profiler resource
4. **Sidecar Container**: Runs alongside the target container, monitoring resource usage and collecting profiles

When a pod is created that matches the selector in a Profiler resource, the webhook injects a sidecar container. This sidecar monitors the target container's resource usage and, when thresholds are exceeded, collects pprof profiles and uploads them to S3.

## 📊 Metrics

The pprof-operator provides comprehensive metrics to monitor its performance and usage. These metrics are exposed via Prometheus endpoints in both the operator and the sidecar containers.

### Operator Metrics

The operator exposes the following metrics on port 8443 (HTTPS) or 8080 (HTTP):

| Metric Name | Type | Description |
|-------------|------|-------------|
| `pprof_operator_sidecars_attached_total` | Counter | Total number of pprof sidecars attached to pods |
| `pprof_operator_sidecars_removed_total` | Counter | Total number of pprof sidecars removed from pods |
| `pprof_operator_active_sidecars` | Gauge | Current number of active pprof sidecars |

### Sidecar Metrics

Each sidecar container exposes the following metrics on port 8080 by default. You can override this via the pod annotation `profiler.pprof.dev/sidecar_metrics_port`. The operator sets the sidecar's `METRICS_ADDR` environment variable accordingly.

| Metric Name | Type | Labels | Description |
|-------------|------|--------|-------------|
| `pprof_sidecar_profiles_generated_total` | Counter | `profile_type` | Total number of pprof profiles generated by type (cpu, heap) |
| `pprof_sidecar_profiles_uploaded_total` | Counter | - | Total number of pprof profiles uploaded to S3 |
| `pprof_sidecar_profile_upload_errors_total` | Counter | - | Total number of errors when uploading profiles to S3 |
| `pprof_sidecar_threshold_exceeded_total` | Counter | `resource_type` | Total number of times resource thresholds were exceeded (cpu, memory) |
| `pprof_sidecar_resource_usage_percent` | Gauge | `resource_type` | Current resource usage as a percentage (cpu, memory) |

### Accessing Metrics

To access the operator metrics:

```sh
# Port-forward the metrics service
kubectl port-forward -n pprof-operator-system svc/controller-manager-metrics-service 8443:8443

# Access metrics (if using HTTPS)
curl -k https://localhost:8443/metrics
```

To access sidecar metrics (default port 8080, or your custom port if set via the `profiler.pprof.dev/sidecar_metrics_port` annotation):

```sh
# Port-forward to a specific pod's sidecar on the default port
kubectl port-forward -n <namespace> <pod-name> 8080:8080
curl http://localhost:8080/metrics

# If you configured a custom metrics port (e.g., 9090):
kubectl port-forward -n <namespace> <pod-name> 9090:9090
curl http://localhost:9090/metrics
```

### Integrating with Prometheus

The operator includes a ServiceMonitor resource that can be used with Prometheus Operator to automatically scrape metrics. To enable this, make sure you have Prometheus Operator installed and the ServiceMonitor CRD available in your cluster.

## 🔒 Security Considerations

The pprof-operator requires certain permissions to function properly:

1. **RBAC Permissions**: The operator needs permissions to watch and modify pods, deployments, and Profiler resources
2. **AWS Credentials**: For uploading profiles to S3, the operator needs AWS credentials
3. **Host Access**: The sidecar container needs access to the Kubernetes metrics API to monitor resource usage

To minimize security risks:
- Use a dedicated IAM role with minimal permissions for S3 access (only PutObject permissions for the specific bucket)
- Store AWS credentials in a Kubernetes secret and ensure it's properly secured
- Consider network policies to restrict the operator's communication
- Review the RBAC permissions in the deployment manifests
- Use TLS for webhook communication (enabled by default)
- Consider using pod security contexts to further restrict the sidecar container

### Performance Impact Considerations

The pprof-operator is designed to have minimal impact on your applications:

1. **Resource Usage**: The sidecar container has low resource requirements (10m CPU, 64Mi memory by default)
2. **Monitoring Overhead**: The sidecar polls the Kubernetes metrics API at configurable intervals (default: 15s)
3. **Profiling Impact**: 
   - CPU profiling adds a small overhead (typically <5%) during the profiling period
   - Memory profiling has negligible impact but may pause the application briefly when collecting heap profiles
   - Profile collection is rate-limited to avoid excessive profiling
4. **Network Traffic**: Profiles are uploaded to S3 only when thresholds are exceeded, not continuously

To minimize performance impact:
- Set appropriate CPU and memory thresholds to avoid excessive profiling
- Configure longer monitoring periods for non-critical applications
- Consider using different thresholds for different environments (dev, staging, prod)
- Monitor the resource usage of the sidecar container itself

## ⚡ Quick Start

Get the operator running and collect your first profiles in a few minutes.

1) Install CRDs and deploy the operator

```sh
# From the repo root
make install

# Deploy the controller (replace the image if you use your own registry)
make deploy IMG=ghcr.io/maulindesai/pprof-operator:latest
```

2) Create AWS credentials secret (for uploading profiles to S3)

Use the provided example and edit with your access key/secret:

```sh
kubectl apply -f config/samples/examples/sample-app/aws-credentials.yaml
```

3) Create a Profiler resource

Create a minimal Profiler (name: profiler-sample). Adjust bucket/region/path as needed.

```yaml
apiVersion: observability.pprof-operator.dev/v1
kind: Profiler
metadata:
  name: profiler-sample
spec:
  cpuThreshold: 70
  memoryThreshold: 80
  monitoringPeriod: 5
  s3Bucket: "your-bucket"
  s3Region: "us-east-1"
  s3PathPrefix: "profiles/sample-app"
  awsCredentialsSecret: "aws-credentials"
```

Apply it:

```sh
kubectl apply -f - <<'EOF'
apiVersion: observability.pprof-operator.dev/v1
kind: Profiler
metadata:
  name: profiler-sample
spec:
  cpuThreshold: 70
  memoryThreshold: 80
  monitoringPeriod: 5
  s3Bucket: "your-bucket"
  s3Region: "us-east-1"
  s3PathPrefix: "profiles/sample-app"
  awsCredentialsSecret: "aws-credentials"
EOF
```

4) Deploy a sample app and annotate it for profiling

You can use the provided sample app, which exposes pprof on :8080. Make sure to include the profiler name annotation.

```sh
# Build and load the sample app image (optional; you can also use your own app)
# docker build -t sample-app:latest config/samples/examples/sample-app

# Deploy the sample app
kubectl apply -f config/samples/examples/sample-app/deployment.yaml

# Add the profiler name annotation required by the webhook (if not already present)
kubectl annotate deploy/pprof-sample-app profiler.pprof.dev/name=profiler-sample --overwrite
```

Required pod annotations:
- profiler.pprof.dev/enable: "true"![logo.jpeg](../../../Downloads/logo.jpeg)
- profiler.pprof.dev/name: "profiler-sample" (must match the Profiler resource name)
- profiler.pprof.dev/target_container: name of your app container (e.g., "app")
- profiler.pprof.dev/scrape_url: pprof base URL (e.g., "http://localhost:8080/debug/pprof")

Optional:
- profiler.pprof.dev/sidecar_metrics_port: "9090" (override sidecar metrics port; default 8080)

5) Verify the sidecar was injected

```sh
kubectl get pods -l app=pprof-sample-app -o=jsonpath='{range .items[*]}{.metadata.name}{"\t"}{range .spec.containers[*]}{.name}{","}{end}{"\n"}{end}'
```

Look for a container named "pprof-sidecar".

6) Access sidecar metrics (Prometheus)

```sh
# Default port
kubectl port-forward deploy/pprof-sample-app 8080:8080
curl http://localhost:8080/metrics

# If you set a custom port via annotation (e.g., 9090)
# kubectl port-forward deploy/pprof-sample-app 9090:9090
# curl http://localhost:9090/metrics
```

7) Check profiles in S3

Profiles are uploaded when CPU/memory thresholds are exceeded under the configured bucket/path.

---

## 🚀 Getting Started

### Prerequisites
- Go version v1.24.0+
- Docker version 17.03+
- kubectl version v1.11.3+
- Access to a Kubernetes v1.11.3+ cluster
- AWS account with S3 access (for storing profiles)

### Installation

#### 1. Clone the repository
```sh
git clone https://github.com/maulindesai/pprof-operator.git
cd pprof-operator
```

#### 2. Build and push the images
Build and push both the operator and sidecar images:

```sh
make docker-build-all docker-push-all IMG=<your-registry>/pprof-operator:tag SIDECAR_IMG=<your-registry>/pprof-sidecar:tag
```

> **NOTE:** Make sure you have permission to push to the specified registry and that the images are accessible from your Kubernetes cluster.

#### 3. Install the CRDs
```sh
make install
```

#### 4. Deploy the operator
```sh
make deploy IMG=<your-registry>/pprof-operator:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin privileges or be logged in as admin.

### Configuration

#### 1. Create AWS credentials secret
The sidecar needs AWS credentials to upload profiles to S3:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: aws-credentials
  namespace: default  # Change to your namespace
type: Opaque
stringData:
  AWS_ACCESS_KEY_ID: "your-access-key"
  AWS_SECRET_ACCESS_KEY: "your-secret-key"
```

Apply the secret:

```sh
kubectl apply -f aws-credentials.yaml
```

#### 2. Create a Profiler resource
Create a Profiler resource to configure which pods to monitor and the thresholds for profiling:

```yaml
apiVersion: observability.pprof-operator.dev/v1
kind: Profiler
metadata:
  name: example-profiler
  namespace: default  # Change to your namespace
spec:
  # CPU threshold in percentage (0-100) that triggers profiling
  cpuThreshold: 70

  # Memory threshold in percentage (0-100) that triggers profiling
  memoryThreshold: 80

  # Monitoring period in seconds to check resource usage (default: 15)
  monitoringPeriod: 15

  # S3 bucket to upload profiles to (required)
  s3Bucket: my-profiles-bucket

  # S3 region (default: us-east-1)
  s3Region: us-west-2

  # S3 path prefix for storing profiles (optional)
  s3PathPrefix: profiles/

  # Option A: Use AWS credentials from a secret (recommended)
  awsCredentialsSecret: aws-credentials

  # Option B: Provide AWS credentials inline (not recommended for production)
  # AWS_ACCESS_KEY_ID: "your-access-key"
  # AWS_SECRET_ACCESS_KEY: "your-secret-key"

  # Optional: Scrape URL configuration for protected pprof endpoints
  scrapURL:
    scrapeURL: "http://localhost:8080/debug/pprof"
    auth:
      type: Basic  # or "None"
      basicAuth:
        username: "admin"
        password: "password"
```

Apply the Profiler resource:

```sh
kubectl apply -f your-profiler.yaml
```

You can also use the sample configuration:

```sh
kubectl apply -k config/samples/
```

> **NOTE**: Make sure to update the sample with your S3 bucket and AWS credentials.

#### Profiler CRD Configuration Reference

The Profiler CRD supports the following configuration options:

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `cpuThreshold` | `int32` | No | - | CPU usage percentage (0-100) that triggers profiling |
| `memoryThreshold` | `int32` | No | - | Memory usage percentage (0-100) that triggers profiling |
| `monitoringPeriod` | `int32` | No | 15 | Interval in seconds to check resource usage |
| `s3Bucket` | `string` | Yes | - | S3 bucket name for storing profiles |
| `s3Region` | `string` | No | us-east-1 | AWS region for the S3 bucket |
| `s3PathPrefix` | `string` | No | - | Path prefix for storing profiles in the S3 bucket |
| `awsCredentialsSecret` | `string` | No | - | Name of the secret containing AWS credentials (if not using inline creds or instance role) |
| `AWS_ACCESS_KEY_ID` | `string` | No | - | Inline AWS access key ID (alternative to using a secret) |
| `AWS_SECRET_ACCESS_KEY` | `string` | No | - | Inline AWS secret access key (alternative to using a secret) |
| `scrapURL` | `ScrapTarget` | No | - | Configuration for scraping pprof endpoints |

##### ScrapTarget Configuration

The `scrapURL` field allows you to configure how the sidecar scrapes pprof endpoints:

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `scrapeURL` | `string` | Yes | - | Base URL for the pprof endpoint (e.g., "http://localhost:8080/debug/pprof") |
| `auth` | `Auth` | No | - | Authentication configuration for the pprof endpoint |

##### Auth Configuration

The `auth` field supports the following authentication methods:

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `type` | `string` | Yes | Basic | Authentication type ("Basic" or "None") |
| `basicAuth` | `BasicAuth` | No | - | Basic authentication configuration |

##### BasicAuth Configuration

For basic authentication, you can provide credentials directly or reference a secret:

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `username` | `string` | No | - | Username for basic authentication |
| `password` | `string` | No | - | Password for basic authentication |
| `secretRef` | `SecretRef` | No | - | Reference to a secret containing credentials |

##### SecretRef Configuration

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `name` | `string` | Yes | - | Name of the secret |
| `namespace` | `string` | No | Same as Profiler | Namespace of the secret |
| `usernameKey` | `string` | No | username | Key in the secret for the username |
| `passwordKey` | `string` | No | password | Key in the secret for the password |

### Uninstallation

#### 1. Delete the Profiler resources
```sh
kubectl delete -k config/samples/
```

#### 2. Delete the CRDs
```sh
make uninstall
```

#### 3. Undeploy the controller
```sh
make undeploy
```

## 📦 Sample Application

The repository includes a sample Go application that demonstrates how to use the pprof-operator:

### What's Included

- **Sample Go Application**: A simple HTTP server with pprof endpoints and functions to simulate CPU and memory load
- **Kubernetes Deployment**: Manifest for deploying the sample application to Kubernetes
- **Profiler Configuration**: Custom Resource for configuring the pprof-operator to monitor the sample application

### Quick Start with Sample App

1. Build and push the sample application image:
   ```sh
   docker build -t <your-registry>/pprof-sample-app:latest config/samples/examples/sample-app/
   docker push <your-registry>/pprof-sample-app:latest
   ```

2. Deploy the application:
   ```sh
   # Update the image in the deployment.yaml file
   sed 's|${SAMPLE_APP_IMAGE}|<your-registry>/pprof-sample-app:latest|g' config/samples/examples/sample-app/deployment.yaml | kubectl apply -f -
   ```

3. Generate load to trigger profiling:
   ```sh
   # Port-forward the service
   kubectl port-forward svc/pprof-sample-app 8080:8080

   # In another terminal, generate CPU load
   curl http://localhost:8080/load/cpu

   # Generate memory load
   curl http://localhost:8080/load/memory
   ```

4. Check the Profiler status:
   ```sh
   kubectl get profiler -o yaml
   ```

### Viewing and Analyzing Profiles

Profiles are uploaded to the configured S3 bucket and can be analyzed using the `go tool pprof` command:

```sh
# Download a profile from S3
aws s3 cp s3://your-bucket/profiles/sample-app/cpu-<timestamp>.pprof ./cpu.pprof

# Analyze the profile with web UI
go tool pprof -http=:8081 ./cpu.pprof

# Or use the interactive terminal UI
go tool pprof ./cpu.pprof
```

#### Profile Analysis Guide

The pprof tool provides several ways to analyze your profiles:

1. **Web UI** (recommended for visual exploration):
   ```sh
   go tool pprof -http=:8081 ./cpu.pprof
   ```
   This opens a web interface with multiple views:
   - **Graph**: Visual call graph showing where time is spent
   - **Flame Graph**: Hierarchical view of call stacks
   - **Top**: Functions sorted by resource consumption
   - **Source**: Annotated source code showing hot spots

2. **Terminal UI** (useful for quick analysis or remote servers):
   ```sh
   go tool pprof ./cpu.pprof
   ```
   Common commands in the interactive terminal:
   - `top`: Show top functions by CPU time
   - `list <function>`: Show source code for a function
   - `web`: Generate and open a SVG call graph (requires Graphviz)
   - `help`: Show all available commands

3. **Comparing Profiles** (to identify changes over time):
   ```sh
   go tool pprof -http=:8081 -diff_base=./cpu-old.pprof ./cpu-new.pprof
   ```
   This highlights differences between two profiles, helping identify regressions.

#### Common Performance Issues to Look For

When analyzing profiles, look for these common patterns:

1. **CPU Profiles**:
   - Functions consuming excessive CPU time
   - Unexpected hot spots in seemingly simple code
   - Excessive garbage collection activity
   - Inefficient algorithms with high complexity

2. **Memory Profiles**:
   - Memory leaks (objects that aren't being garbage collected)
   - Excessive allocations in hot code paths
   - Large temporary objects that could be avoided
   - String concatenation in loops (consider using strings.Builder)

3. **Block Profiles** (if enabled):
   - Excessive lock contention
   - Long-held locks blocking other goroutines
   - Inefficient synchronization patterns

#### Advanced Analysis Techniques

For more advanced analysis:

1. **Differential Profiling**:
   Compare profiles before and after code changes to identify regressions.

2. **Continuous Profiling**:
   Set up regular profiling to track performance over time and identify gradual degradations.

3. **Custom Visualization**:
   Export profiles to other formats for custom analysis:
   ```sh
   go tool pprof -proto ./cpu.pprof > profile.pb.gz
   ```

4. **Integration with Monitoring**:
   Correlate profile data with metrics from your monitoring system to understand the context of performance issues.

For more information on pprof, see the [official documentation](https://github.com/google/pprof/blob/master/doc/README.md).

## 📦 Distribution

There are multiple ways to distribute and install the pprof-operator:

### YAML Bundle

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=<your-registry>/pprof-operator:tag
```

> **NOTE:** This generates an 'install.yaml' file in the dist directory containing all resources needed to install the project.

2. Users can install the operator with:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/pprof-operator/<tag or branch>/dist/install.yaml
```

### Helm Chart

1. Build the chart using the optional helm plugin:

```sh
kubebuilder edit --plugins=helm/v1-alpha
```

2. A chart will be generated under 'dist/chart' that users can install.

> **NOTE:** When updating the project, remember to update the Helm Chart using the same command to sync changes. For webhooks, use the '--force' flag and manually reapply any custom configuration.

## 🔧 Troubleshooting

### Common Issues

#### 1. Sidecar Not Injected
- Check if the pod has the correct labels matching the Profiler selector
- Verify the webhook is running: `kubectl get pods -n <operator-namespace>`
- Check webhook logs: `kubectl logs -n <operator-namespace> <webhook-pod-name>`
- Ensure the pod is created after the Profiler resource

#### 2. Profiles Not Being Collected
- Check sidecar logs: `kubectl logs -n <app-namespace> <pod-name> -c pprof-sidecar`
- Verify CPU/Memory thresholds are set appropriately
- Ensure the target container exposes pprof endpoints

#### 3. Profiles Not Uploading to S3
- Verify AWS credentials are correct
- Check S3 bucket permissions
- Ensure the sidecar has network access to AWS S3
- Check for any errors in the sidecar logs

#### 4. Webhook Certificate Issues
- Verify certificates are properly mounted
- Check certificate expiration
- Regenerate certificates if needed: `kubectl delete secret webhook-server-cert -n <operator-namespace>`

### Debugging Tips

#### Enable Verbose Logging
Set the log level to debug in the operator deployment:
```sh
kubectl patch deployment pprof-operator-controller-manager -n pprof-operator-system --type='json' -p='[{"op": "replace", "path": "/spec/template/spec/containers/0/args", "value": ["--health-probe-bind-address=:8081", "--metrics-bind-address=127.0.0.1:8080", "--leader-elect", "--zap-log-level=debug"]}]'
```

#### Check Kubernetes Events
View events related to the operator and target pods:
```sh
kubectl get events -n <namespace>
```

#### Verify RBAC Permissions
Ensure the operator service account has the necessary permissions:
```sh
kubectl auth can-i <verb> <resource> --as=system:serviceaccount:<namespace>:<serviceaccount>
```

## 👥 Contributing

We welcome contributions to the pprof-operator! Here's how you can help:

### Ways to Contribute

#### 🐛 Report Issues
If you find a bug or have a feature request, please [open an issue](https://github.com/maulindesai/pprof-operator/issues/new) with:
- A clear description of the problem
- Steps to reproduce
- Expected vs. actual behavior
- Your environment details

#### 🔀 Submit Pull Requests
1. Fork the repository
2. Create a new branch: `git checkout -b feature/your-feature-name`
3. Make your changes
4. Run tests: `make test`
5. Submit a pull request

#### 📚 Development Guidelines
- Follow [Go coding standards](https://github.com/golang/go/wiki/CodeReviewComments)
- Add unit tests for new functionality
- Update documentation for any changes
- Run `make test` to ensure all tests pass
- Run `make lint` to check for code quality issues

#### 🔍 Code Review Process
- All pull requests require at least one review
- Address any comments or feedback from reviewers
- Once approved, a maintainer will merge your changes

> **TIP:** Run `make help` for more information on all available `make` targets

For more information on Kubernetes operators, see the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## 📚 Additional Resources

### Related Projects

- [Kubernetes](https://kubernetes.io/) - The container orchestration platform
- [Kubebuilder](https://book.kubebuilder.io/) - Framework for building Kubernetes APIs
- [pprof](https://github.com/google/pprof) - The Go profiling tool
- [Prometheus](https://prometheus.io/) - Monitoring system that pairs well with pprof-operator

### Further Reading

- [Profiling Go Programs](https://blog.golang.org/pprof) - Official Go blog post on profiling
- [Continuous Profiling in Go](https://medium.com/@tvii/continuous-profiling-in-go-b2ac0e9e8b44) - Article on continuous profiling
- [Kubernetes Operators Pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/) - Learn more about the operator pattern

## License

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
