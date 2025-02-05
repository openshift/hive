<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->
**Table of Contents**  *generated with [DocToc](https://github.com/thlorenz/doctoc)*

- [Hive Metrics](#hive-metrics)
    - [Optional Metrics](#optional-metrics)
      - [Duration-based Metrics](#duration-based-metrics)
      - [Metrics with Optional Cluster Deployment labels](#metrics-with-optional-cluster-deployment-labels)
    - [List of all Hive metrics](#list-of-all-hive-metrics)
      - [Hive Operator metrics](#hive-operator-metrics)
      - [Metrics reported by all controllers](#metrics-reported-by-all-controllers)
      - [ClusterDeployment controller metrics](#clusterdeployment-controller-metrics)
      - [ClusterProvision controller metrics](#clusterprovision-controller-metrics)
      - [ClusterDeprovision controller metrics](#clusterdeprovision-controller-metrics)
      - [ClusterPool controller metrics](#clusterpool-controller-metrics)
      - [Metrics controller metrics](#metrics-controller-metrics)
    - [Managed DNS Metrics](#managed-dns-metrics)
    - [Example: Configure metricsConfig](#example-configure-metricsconfig)
    - [Frequently Asked Questions](#frequently-asked-questions)
      - [How can I leverage hive metrics to point to the offending cluster?](#how-can-i-leverage-hive-metrics-to-point-to-the-offending-cluster)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

# Hive Metrics

Hive publishes metrics, which can help admins to monitor the Hive operations. Most of these metrics are always published; a few are [optional](#optional-metrics).

### Optional Metrics

#### Duration-based Metrics

Certain duration metrics use labels whose values are by nature unbounded (e.g. ClusterDeployment name and namespace).
These are not logged by default as this can overwhelm the prometheus database (see https://prometheus.io/docs/practices/naming/#labels).
Admins can use `HiveConfig.Spec.MetricsConfig.MetricsWithDuration` to opt for logging such metrics only when the duration exceeds the configured threshold. An example of this can be found [here](#example-configure-metricsconfig).
Check the metrics labelled as `Optional` in the [list below](#optional-metrics) to see affected metrics. 

#### Metrics with Optional Cluster Deployment labels

Most metrics are not observed per cluster deployment name or namespace, so Hive allows for admins to define the labels to look for when reporting these metrics.

Opt into them via `HiveConfig.Spec.MetricsConfig.AdditionalClusterDeploymentLabels`, which accepts a map of the label you would want on your metrics, to the label found on ClusterDeployment.
For example, including `{"ocp_major_version": "hive.openshift.io/version-major"}` will cause affected metrics to include a label key ocp_major_version with the value from the `hive.openshift.io/version-major` ClusterDeployment label -- e.g. "4".
Every metric that allows optional labels will always have all the labels mentioned present. If the corresponding Cluster Deployment label is not present then the metric label will report its value as "unspecified".

The hive operator will panic if provided additional labels overlap with the fixed labels of the corresponding metric. Please refer to the fixed labels of each metric [here](#list-of-all-hive-metrics).

Note: It is up to the cluster admins to be mindful of cardinality and ensure these labels are not too specific, like cluster id, otherwise it can negatively impact your observability system's performance

### List of all Hive metrics

#### Hive Operator metrics
These metrics are observed by the Hive Operator. None of these are optional.

|           Metric Name           | Optional Label Support | Fixed Labels              |
|:-------------------------------:|:----------------------:|---------------------------|
|   hive_hiveconfig_conditions    |           N            | {"condition", "reason"}   |
| hive_operator_reconcile_seconds |           N            | {"controller", "outcome"} |

#### Metrics reported by all controllers
These metrics are observed by all Hive Controllers. None of these are optional.

|                Metric Name                | Optional Label Support | Fixed Labels                                             |
|:-----------------------------------------:|:----------------------:|----------------------------------------------------------|
|      hive_kube_client_requests_total      |           N            | {"controller", "method", "resource", "remote", "status"} |
|     hive_kube_client_request_seconds      |           N            | {"controller", "method", "resource", "remote", "status"} |
| hive_kube_client_requests_cancelled_total |           N            | {"controller", "method", "resource", "remote"}           |

#### ClusterDeployment controller metrics
These metrics are observed while processing ClusterDeployments. None of these are optional.

|                       Metric Name                        | Optional Label Support | Fixed Labels                                     |
|:--------------------------------------------------------:|:----------------------:|--------------------------------------------------|
|   hive_cluster_deployment_install_job_duration_seconds   |           N            | {}                                               |
|    hive_cluster_deployment_install_job_delay_seconds     |           N            | {}                                               |
|    hive_cluster_deployment_imageset_job_delay_seconds    |           N            | {}                                               |
|        hive_cluster_deployment_dns_delay_seconds         |           N            | {}                                               |
|    hive_cluster_deployment_completed_install_restart     |           Y            | {}                                               |
|          hive_cluster_deployments_created_total          |           Y            | {}                                               |
|         hive_cluster_deployments_installed_total         |           Y            | {}                                               |
|          hive_cluster_deployments_deleted_total          |           Y            | {}                                               |
| hive_cluster_deployments_provision_failed_terminal_total |           Y            | {"clusterpool_namespacedname", "failure_reason"} |

#### ClusterProvision controller metrics
These metrics are observed while processing ClusterProvisions. None of these are optional.

|                  Metric Name                  | Optional Label Support | Fixed Labels                                                            |
|:---------------------------------------------:|:----------------------:|-------------------------------------------------------------------------|
|     hive_cluster_provision_results_total      |           Y            | {"result"}                                                              |
|              hive_install_errors              |           Y            | {"reason"}                                                              |
| hive_cluster_deployment_install_failure_total |           Y            | {"platform", "region", "cluster_version", "workers", "install_attempt"} |
| hive_cluster_deployment_install_success_total |           Y            | {"platform", "region", "cluster_version", "workers", "install_attempt"} |

#### ClusterDeprovision controller metrics
These metrics are observed while processing ClusterDeprovisions. None of these are optional.

|                      Metric Name                       | Optional Label Support | Fixed Labels |
|:------------------------------------------------------:|:----------------------:|--------------|
| hive_cluster_deployment_uninstall_job_duration_seconds |           N            | {}           |

#### ClusterPool controller metrics
These metrics are observed while processing ClusterPools. None of these are optional.

|                    Metric Name                    | Optional Label Support | Fixed Labels                                  |
|:-------------------------------------------------:|:----------------------:|-----------------------------------------------|
|  hive_clusterpool_clusterdeployments_assignable   |           N            | {"clusterpool_namespace", "clusterpool_name"} |
|    hive_clusterpool_clusterdeployments_claimed    |           N            | {"clusterpool_namespace", "clusterpool_name"} |
|   hive_clusterpool_clusterdeployments_deleting    |           N            | {"clusterpool_namespace", "clusterpool_name"} |
|  hive_clusterpool_clusterdeployments_installing   |           N            | {"clusterpool_namespace", "clusterpool_name"} |
|   hive_clusterpool_clusterdeployments_unclaimed   |           N            | {"clusterpool_namespace", "clusterpool_name"} |
|    hive_clusterpool_clusterdeployments_standby    |           N            | {"clusterpool_namespace", "clusterpool_name"} |
|     hive_clusterpool_clusterdeployments_stale     |           N            | {"clusterpool_namespace", "clusterpool_name"} |
|    hive_clusterpool_clusterdeployments_broken     |           N            | {"clusterpool_namespace", "clusterpool_name"} |
| hive_clusterpool_stale_clusterdeployments_deleted |           N            | {"clusterpool_namespace", "clusterpool_name"} |
|    hive_clusterclaim_assignment_delay_seconds     |           N            | {"clusterpool_namespace", "clusterpool_name"} |

#### Metrics controller metrics
These metrics are accumulated across all instance of that type.
Some of these metrics are optional and the admin can opt for logging them via `HiveConfig.Spec.MetricsConfig.MetricsWithDuration`

|                          Metric Name                           | Optional Label Support | Optional | Fixed Labels                                                                                                    |
|:--------------------------------------------------------------:|:----------------------:|:--------:|-----------------------------------------------------------------------------------------------------------------|
|                    hive_cluster_deployments                    |           N            |    N     | {"cluster_type", "age_lt", "power_state"}                                                                       |
|               hive_cluster_deployments_installed               |           N            |    N     | {"cluster_type", "age_lt"}                                                                                      |
|              hive_cluster_deployments_uninstalled              |           N            |    N     | {"cluster_type", "age_lt", "uninstalled_gt"}                                                                    |
|            hive_cluster_deployments_deprovisioning             |           N            |    N     | {"cluster_type", "age_lt", "deprovisioning_gt"}                                                                 |
|              hive_cluster_deployments_conditions               |           N            |    N     | {"cluster_type", "age_lt", "condition"}                                                                         |
|                       hive_install_jobs                        |           N            |    N     | {"cluster_type", "state"}                                                                                       |
|                      hive_uninstall_jobs                       |           N            |    N     | {"cluster_type", "state"}                                                                                       |
|                       hive_imageset_jobs                       |           N            |    N     | {"cluster_type", "state"}                                                                                       |
|              hive_selectorsyncset_clusters_total               |           N            |    N     | {"name"}                                                                                                        |
|         hive_selectorsyncset_clusters_unapplied_total          |           N            |    N     | {"name"}                                                                                                        |
|                      hive_syncsets_total                       |           N            |    N     | {}                                                                                                              |
|                 hive_syncsets_unapplied_total                  |           N            |    N     | {}                                                                                                              |
|      hive_cluster_deployment_deprovision_underway_seconds      |           N            |    N     | {"cluster_deployment", "namespace", "cluster_type"}                                                             |
|                hive_clustersync_failing_seconds                |           Y            |    Y     | {"namespaced_name", "unreachable"}                                                                              |
|    hive_cluster_deployments_hibernation_transition_seconds     |           N            |    Y     | {"cluster_version", "platform", "cluster_pool_namespace", "cluster_pool_name"}                                  |
|      hive_cluster_deployments_running_transition_seconds       |           N            |    Y     | {"cluster_version", "platform", "cluster_pool_namespace", "cluster_pool_name"}                                  |
|           hive_cluster_deployments_stopping_seconds            |           N            |    Y     | {"cluster_deployment_namespace", "cluster_deployment", "platform", "cluster_version", "cluster_pool_namespace"} |
|           hive_cluster_deployments_resuming_seconds            |           N            |    Y     | {"cluster_deployment_namespace", "cluster_deployment", "platform", "cluster_version", "cluster_pool_namespace"} |
| hive_cluster_deployments_waiting_for_cluster_operators_seconds |           N            |    Y     | {"cluster_deployment_namespace", "cluster_deployment", "platform", "cluster_version", "cluster_pool_namespace"} |
|               hive_controller_reconcile_seconds                |           N            |    N     | {"controller", "outcome"}                                                                                       |
|             hive_cluster_deployment_syncset_paused             |           N            |    N     | {"cluster_deployment", "namespace", "cluster_type"}                                                             |
|       hive_cluster_deployment_provision_underway_seconds       |           N            |    N     | {"cluster_deployment", "namespace", "cluster_type", "condition", "reason", "platform", "image_set"}             |
|  hive_cluster_deployment_provision_underway_install_restarts   |           N            |    N     | {"cluster_deployment", "namespace", "cluster_type", "condition", "reason", "platform", "image_set"}             |

### Managed DNS Metrics
These are specific to the [Managed DNS flow](using-hive.md#managed-dns-1), and are probably interesting only to developers.
Not optional.

|           Metric Name               | Optional Label Support | Fixed Labels       |
|:-----------------------------------:|:----------------------:|--------------------|
|   hive_managed_dns_scrape_seconds   |           N            | {"managed_domain"} |
| hive_managed_dns_subdomains_scraped |           N            | {"managed_domain"} |

### Example: Configure metricsConfig

```sh
oc edit hiveconfig -n hive
```

```yaml
spec:
  metricsConfig:
    metricsWithDuration:
      - name: <duration metric type>
        duration: <min duration>
```

Ex. Register a hive_clustersync_failing_seconds metric.

```yaml
spec:
  metricsConfig:
    metricsWithDuration:
      - name: currentClusterSyncFailing
        duration: 1h
```

|                           Metric name                          |    Duration metric type   |
|:--------------------------------------------------------------:|:-------------------------:|
|            hive_cluster_deployments_stopping_seconds           |      currentStopping      |
|            hive_cluster_deployments_resuming_seconds           |      currentResuming      |
| hive_cluster_deployments_waiting_for_cluster_operators_seconds |    currentWaitingForCO    |
|                hive_clustersync_failing_seconds                | currentClusterSyncFailing |
|     hive_cluster_deployments_hibernation_transition_seconds    |    cumulativeHibernated   |
|       hive_cluster_deployments_running_transition_seconds      |     cumulativeResumed     |

### Frequently Asked Questions

#### How can I leverage hive metrics to point to the offending cluster?
Metrics are meant for monitoring and observing. While you can leverage Alertmanager to throw alerts, it is really not recommended to rely on prometheus metrics to provide the cluster ID.

For this to work, the hive metric would need to publish a label with the cluster identifying information. Since every metric is stored as a map with dimensions equivalent to the labels per its definition,
the storage taken up by the metric exponentially increases with the number of labels. Hive can manage hundreds, if not thousands of clusters at a time, so high-cardinality labels like cluster ID/name/namespace
are capable of bringing down the hive instance in certain situations.
While we publish the cluster identifying label with certain metrics that are reported infrequently enough to not cause an issue, Hive does not recommend relying on hive metrics for tracing the clusters.