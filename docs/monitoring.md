# Use OpenShift Cluster Monitoring to expose Hive metrics

OpenShift ships with monitoring stack that can be used to export various
Hive metrics.

## Enabling monitoring for user-defined projects

Follow the guidelines as defined in [OpenShift documentation][user-projects-monitoring]

ServiceMonitors can be created for any or all of hive-operator, hive-controllers, and hive-clustersync.
For example:

```yaml
---
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: hive-operator
spec:
  endpoints:
  - interval: 30s
    path: /metrics
    port: metrics
    scheme: http
    metricRelabelings:
    - sourceLabels: [__name__]
      regex: '^rest_client_.*'
      action: drop
  selector:
    matchLabels:
      control-plane: hive-operator
---
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: hive-controllers
spec:
  endpoints:
  - interval: 30s
    path: /metrics
    port: metrics
    scheme: http
    metricRelabelings:
    - sourceLabels: [__name__]
      regex: '^rest_client_.*'
      action: drop
  selector:
    matchLabels:
      control-plane: controller-manager
---
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: hive-clustersync
spec:
  endpoints:
  - interval: 30s
    path: /metrics
    port: metrics
    scheme: http
    metricRelabelings:
    - sourceLabels: [__name__]
      regex: '^rest_client_.*'
      action: drop
  selector:
    matchLabels:
      control-plane: clustersync
```

[user-projects-monitoring]: https://docs.openshift.com/container-platform/4.12/monitoring/enabling-monitoring-for-user-defined-projects.html