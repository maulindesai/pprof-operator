# pprof-sidecar

A sidecar container for collecting pprof profiles from target containers when CPU or memory thresholds are exceeded.

## Overview

The pprof-sidecar is designed to work with the pprof-operator to automatically collect performance profiles from containers when resource usage exceeds configured thresholds. The collected profiles are uploaded to AWS S3 for later analysis.

## How it works

1. The sidecar container is injected into pods by the pprof-operator
2. It monitors the target container's CPU and memory usage
3. When usage exceeds the configured thresholds, it collects a pprof profile
4. The profile is uploaded to AWS S3 with metadata about why it was triggered

## Configuration

The sidecar is configured through environment variables, which are automatically set by the pprof-operator based on the Profiler CRD:

| Environment Variable | Description | Default |
|----------------------|-------------|---------|
| TARGET_CONTAINER | Name of the container to monitor | (required) |
| TARGET_PID | PID of the process to monitor (optional, will be auto-detected) | (auto-detected) |
| CPU_THRESHOLD | CPU usage percentage that triggers profiling | 80 |
| MEMORY_THRESHOLD | Memory usage percentage that triggers profiling | 80 |
| PROFILE_DURATION | Duration in seconds to collect the profile | 30 |
| S3_BUCKET | S3 bucket to upload profiles to | (required) |
| S3_REGION | AWS region for the S3 bucket | us-east-1 |
| S3_PATH_PREFIX | Path prefix for storing profiles in the S3 bucket | (empty) |
| AWS_ACCESS_KEY_ID | AWS access key ID | (from secret) |
| AWS_SECRET_ACCESS_KEY | AWS secret access key | (from secret) |
| METRICS_ADDR | Address the sidecar exposes Prometheus metrics on | :8080 (set by operator; can be overridden via pod annotation `profiler.pprof.dev/sidecar_metrics_port`) |

## Usage

The sidecar is automatically injected by the pprof-operator when you create a Profiler resource:

```yaml
apiVersion: observability.pprof-operator.dev/v1
kind: Profiler
metadata:
  name: example-profiler
spec:
  selector:
    matchLabels:
      app: my-application
  targetContainer: app
  cpuThreshold: 70
  memoryThreshold: 80
  profileDuration: 30
  s3Bucket: my-profiles-bucket
  s3Region: us-west-2
  s3PathPrefix: profiles/
  awsCredentialsSecret: aws-credentials
```

## Building

To build the sidecar container:

```bash
make build-sidecar
```

## Limitations

- The sidecar currently only supports CPU and heap profiles
- The target container must expose the pprof HTTP endpoint
- The sidecar needs access to the target container's process metrics