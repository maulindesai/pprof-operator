# pprof-operator

A Kubernetes operator for automatically collecting pprof profiles from applications when CPU or memory thresholds are exceeded.

## Description

The pprof-operator is designed to help developers and operators diagnose performance issues in their applications running on Kubernetes. It works by:

1. Monitoring containers for CPU and memory usage
2. Automatically collecting pprof profiles when thresholds are exceeded
3. Uploading the profiles to AWS S3 for later analysis

The operator consists of two main components:
- **Controller**: Manages the Profiler CRD and injects the sidecar container into target pods
- **Sidecar**: Monitors the target container and collects profiles when thresholds are exceeded

## Getting Started

### Prerequisites
- go version v1.24.0+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.

### To Deploy on the cluster
**Build and push both the operator and sidecar images:**

```sh
make docker-build-all docker-push-all IMG=<some-registry>/pprof-operator:tag SIDECAR_IMG=<some-registry>/pprof-sidecar:tag
```

**NOTE:** These images ought to be published in the personal registry you specified.
And it is required to have access to pull the images from the working environment.
Make sure you have the proper permission to the registry if the above commands don’t work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/pprof-operator:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

**Create a Profiler resource**

Create a Profiler resource to configure which pods to monitor and the thresholds for profiling:

```yaml
apiVersion: observability.pprof-operator.dev/v1
kind: Profiler
metadata:
  name: example-profiler
spec:
  # Select pods to monitor by labels
  selector:
    matchLabels:
      app: my-application
  # Target container to monitor
  targetContainer: app
  # CPU threshold in percentage (0-100) that triggers profiling
  cpuThreshold: 70
  # Memory threshold in percentage (0-100) that triggers profiling
  memoryThreshold: 80
  # Duration in seconds to collect the profile
  profileDuration: 30
  # S3 bucket to upload profiles to
  s3Bucket: my-profiles-bucket
  # S3 region
  s3Region: us-west-2
  # S3 path prefix for storing profiles
  s3PathPrefix: profiles/
  # AWS credentials secret name
  awsCredentialsSecret: aws-credentials
```

Apply the Profiler resource:

```sh
kubectl apply -f profiler.yaml
```

You can also apply the sample from the config/samples:

```sh
kubectl apply -k config/samples/
```

> **NOTE**: Make sure to update the sample with your S3 bucket and AWS credentials.

**Create AWS credentials secret**

The sidecar needs AWS credentials to upload profiles to S3. Create a secret with your AWS credentials:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: aws-credentials
type: Opaque
stringData:
  AWS_ACCESS_KEY_ID: "your-access-key"
  AWS_SECRET_ACCESS_KEY: "your-secret-key"
```

Apply the secret:

```sh
kubectl apply -f aws-credentials.yaml
```

Make sure the `awsCredentialsSecret` field in your Profiler resource matches the name of this secret.

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

## Project Distribution

Following the options to release and provide this solution to the users.

### By providing a bundle with all YAML files

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=<some-registry>/pprof-operator:tag
```

**NOTE:** The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without its
dependencies.

2. Using the installer

Users can just run 'kubectl apply -f <URL for YAML BUNDLE>' to install
the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/pprof-operator/<tag or branch>/dist/install.yaml
```

### By providing a Helm Chart

1. Build the chart using the optional helm plugin

```sh
kubebuilder edit --plugins=helm/v1-alpha
```

2. See that a chart was generated under 'dist/chart', and users
can obtain this solution from there.

**NOTE:** If you change the project, you need to update the Helm Chart
using the same command above to sync the latest changes. Furthermore,
if you create webhooks, you need to use the above command with
the '--force' flag and manually ensure that any custom configuration
previously added to 'dist/chart/values.yaml' or 'dist/chart/manager/manager.yaml'
is manually re-applied afterwards.

## Contributing
// TODO(user): Add detailed information on how you would like others to contribute to this project

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

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
