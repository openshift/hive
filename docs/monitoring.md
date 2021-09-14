# Use OpenShift Cluster Monitoring to expose Hive metrics

OpenShift ships with monitoring stack that can be used to export various
Hive metrics.

## Enabling monitoring for user-defined projects

Follow the guidelines as defined in [OpenShift documentation][user-projects-monitoring]

NOTE: If you do not want to enable monitoring for all user-defined projects, you can
still enable metrics for hive controllers by making sure the target namespace in HiveConfig
is `openshift-hive`.

## Enabling export of metrics

Update the HiveConfig so that the hive-operator creates the necessary resources for
monitoring.

```yaml
## hiveconfig
spec:
    exportMetrics: true
```

[user-projects-monitoring]: https://docs.openshift.com/container-platform/4.8/monitoring/enabling-monitoring-for-user-defined-projects.html