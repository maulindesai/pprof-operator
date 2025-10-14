pprof-operator Helm Chart

This directory contains the official Helm chart for deploying the pprof-operator.

Directory layout
- charts/pprof-operator/Chart.yaml: Chart metadata
- charts/pprof-operator/values.yaml: Default configuration values
- charts/pprof-operator/templates/: Kubernetes manifests rendered by Helm

Prerequisites
- Kubernetes 1.22+
- Helm 3.10+
- (Optional) cert-manager if you enable webhook or metrics TLS

Quick install
- Create a namespace (optional)
  kubectl create ns pprof-operator
- Install the chart
  helm upgrade --install pprof-operator charts/pprof-operator -n pprof-operator --create-namespace

Configuration (values.yaml)
The chart is configured via values. You can:
- Use --set for quick overrides
- Provide one or more values files with -f/--values
- Combine both; later flags override earlier ones

Values reference

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| controllerManager.replicas | int | `1` | Number of controller replicas |
| controllerManager.container.image.repository | string | `"ghcr.io/maulindesai/pprof-controller"` | Controller image repository |
| controllerManager.container.image.tag | string | `"latest"` | Controller image tag |
| controllerManager.container.args | list[string] | `["--leader-elect","--metrics-bind-address=:8443","--health-probe-bind-address=:8081"]` | Container args (enable leader election, metrics, health probes) |
| controllerManager.container.resources | object | `{limits:{cpu:"500m",memory:"128Mi"},requests:{cpu:"10m",memory:"64Mi"}}` | Resource requests/limits for the controller |
| controllerManager.container.livenessProbe | object | `{httpGet:{path:"/healthz",port:8081},initialDelaySeconds:15,periodSeconds:20}` | Liveness probe configuration |
| controllerManager.container.readinessProbe | object | `{httpGet:{path:"/readyz",port:8081},initialDelaySeconds:5,periodSeconds:10}` | Readiness probe configuration |
| controllerManager.container.securityContext | object | `{allowPrivilegeEscalation:false,capabilities:{drop:["ALL"]}}` | Container security context |
| controllerManager.securityContext | object | `{runAsNonRoot:true,seccompProfile:{type:"RuntimeDefault"}}` | Pod-level security context |
| controllerManager.terminationGracePeriodSeconds | int | `10` | Pod termination grace period (seconds) |
| controllerManager.serviceAccountName | string | `"pprof-operator-controller-manager"` | Name of ServiceAccount to use |
| rbac.enable | bool | `true` | Create RBAC resources required by the controller |
| crd.enable | bool | `true` | Install the Profiler CRD with the release |
| crd.keep | bool | `true` | Keep CRDs on helm uninstall (prevents accidental CR deletion) |
| metrics.enable | bool | `true` | Expose controller metrics on a Service |
| webhook.enable | bool | `true` | Create webhook Service and MutatingWebhookConfiguration |
| prometheus.enable | bool | `false` | Create a ServiceMonitor for Prometheus Operator |
| certmanager.enable | bool | `true` | Use cert-manager to provision TLS certs for webhook/metrics |
| networkPolicy.enable | bool | `false` | Create restrictive NetworkPolicies for webhook and metrics access |

Using values files
- Single file
  helm upgrade --install pprof-operator charts/pprof-operator -n pprof-operator -f my-values.yaml

- Multiple files (later files override earlier ones)
  helm upgrade --install pprof-operator charts/pprof-operator -n pprof-operator -f values.yaml -f values-prod.yaml

- Mix files and --set (the last occurrence wins)
  helm upgrade --install pprof-operator charts/pprof-operator -n pprof-operator \
    -f values.yaml \
    -f values-prod.yaml \
    --set controllerManager.container.image.tag=v1.2.3

Examples
- Install without CRDs (useful if CRDs are managed separately):
  helm upgrade --install pprof-operator charts/pprof-operator -n pprof-operator --set crd.enable=false
- Enable Prometheus ServiceMonitor:
  helm upgrade --install pprof-operator charts/pprof-operator -n pprof-operator --set prometheus.enable=true
- Use a custom image repo and tag via a values file:
  cat > values-prod.yaml <<'EOF'
  controllerManager:
    container:
      image:
        repository: ghcr.io/my-org/pprof-controller
        tag: v1.0.0
  EOF
  helm upgrade --install pprof-operator charts/pprof-operator -n pprof-operator -f values-prod.yaml

Upgrade
- To upgrade to a new version:
  helm upgrade pprof-operator charts/pprof-operator -n pprof-operator

Uninstall
- Remove the release (keeps CRDs by default):
  helm uninstall pprof-operator -n pprof-operator
- To also remove CRDs, set keep=false and uninstall:
  helm upgrade --install pprof-operator charts/pprof-operator -n pprof-operator --set crd.keep=false
  helm uninstall pprof-operator -n pprof-operator

Notes
- If you enable webhooks or metrics with TLS and do not use cert-manager, you will need to provide certificates manually.
- The chart was generated from the project manifests and may need updates when APIs change; keep values consistent with your environment.
