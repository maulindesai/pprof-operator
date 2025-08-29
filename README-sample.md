# Testing pprof-operator with a Sample Application

This repository includes a sample Go application that demonstrates how to use the pprof-operator to automatically collect profiles when CPU or memory thresholds are exceeded.

## What's Included

- **Sample Go Application**: A simple HTTP server with pprof endpoints and functions to simulate CPU and memory load
- **Dockerfile**: For building the sample application container
- **Kubernetes Deployment**: Manifest for deploying the sample application to Kubernetes
- **Profiler Configuration**: Custom Resource for configuring the pprof-operator to monitor the sample application

## Directory Structure

```
sample-app/
├── main.go              # Sample Go application with pprof enabled
├── Dockerfile           # For building the container image
├── deployment.yaml      # Kubernetes deployment manifest
├── profiler.yaml        # Profiler custom resource configuration
└── README.md            # Detailed instructions for the sample application
```

## Quick Start

1. Build and push the sample application image:
   ```sh
   docker build -t <your-registry>/pprof-sample-app:latest sample-app/
   docker push <your-registry>/pprof-sample-app:latest
   ```

2. Deploy the application and profiler:
   ```sh
   # Update the image in the deployment.yaml file
   sed 's|${SAMPLE_APP_IMAGE}|<your-registry>/pprof-sample-app:latest|g' sample-app/deployment.yaml | kubectl apply -f -
   
   # Create AWS credentials secret
   kubectl create secret generic aws-credentials \
     --from-literal=AWS_ACCESS_KEY_ID=<your-access-key> \
     --from-literal=AWS_SECRET_ACCESS_KEY=<your-secret-key>
   
   # Deploy the Profiler resource
   kubectl apply -f config/sample/observability_v1_profiler.yaml
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
   kubectl get profiler sample-app-profiler -o yaml
   ```

For more detailed instructions, see the [sample application README](config/samples/examples/sample-app/README.md).

## How It Works

1. The sample application exposes pprof endpoints at `/debug/pprof/`
2. The Profiler resource is configured to:
   - Monitor pods with the label `app: pprof-sample-app`
   - Collect profiles when CPU usage exceeds 70% or memory usage exceeds 80%
   - Scrape profiles from the pprof endpoint at `http://localhost:8080/debug/pprof`
3. When thresholds are exceeded, the pprof-operator injects a sidecar container that:
   - Monitors the target container's resource usage
   - Collects profiles when thresholds are exceeded
   - Uploads the profiles to S3

## Viewing Profiles

Profiles are uploaded to the configured S3 bucket and can be analyzed using the `go tool pprof` command:

```sh
# Download a profile from S3
aws s3 cp s3://profiler-bucket/profiles/sample-app/cpu-<timestamp>.pprof ./cpu.pprof

# Analyze the profile
go tool pprof -http=:8081 ./cpu.pprof
```

This will open a web interface where you can explore the profile data.