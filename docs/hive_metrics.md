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
      - [ClusterPool controller metrics](#clusterpool-controller-metrics)
      - [Metrics controller metrics](#metrics-controller-metrics)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

# Hive Metrics

Hive publishes metrics, which can help admins to monitor the Hive operations. Most of these metrics are always published; a few are [optional](#optional-metrics).

### Optional Metrics

#### Duration-based Metrics

Certain duration metrics use labels whose values are by nature unbounded (e.g. ClusterDeployment name and namespace).
These are not logged by default as this can overwhelm the prometheus database (see https://prometheus.io/docs/practices/naming/#labels).
Admins can use `HiveConfig.Spec.MetricsConfig.MetricsWithDuration` to opt for logging such metrics only when the duration exceeds the configured threshold.
Check the metrics labelled as `Optional` in the [list below](#optional-metrics) to see affected metrics. 

#### Metrics with Optional Cluster Deployment labels

Most metrics are not observed per cluster deployment name or namespace, so Hive allows for admins to define the labels to look for when reporting these metrics.

Opt into them via `HiveConfig.Spec.MetricsConfig.AdditionalClusterDeploymentLabels`, which accepts a map of the label you would want on your metrics, to the label found on ClusterDeployment.
For example, including `{"ocp_major_version": "hive.openshift.io/version-major"}` will cause affected metrics to include a label key ocp_major_version with the value from the `hive.openshift.io/version-major` ClusterDeployment label -- e.g. "4".
Every metric that allows optional labels will always have all the labels mentioned present. If the corresponding Cluster Deployment label is not present then the metric label will report its value as "unspecified".

Note: It is up to the cluster admins to be mindful of cardinality and ensure these labels are not too specific, like cluster id, otherwise it can negatively impact your observability system's performance

### List of all Hive metrics

#### Hive Operator metrics
These metrics are observed by the Hive Operator. None of these are optional.

|           Metric Name           | Optional Label Support |
|:-------------------------------:|:----------------------:|
|   hive_hiveconfig_conditions    |           N            |
| hive_operator_reconcile_seconds |           N            |

#### Metrics reported by all controllers
These metrics are observed by all Hive Controllers. None of these are optional.

|                Metric Name                | Optional Label Support |
|:-----------------------------------------:|:----------------------:|
|      hive_kube_client_requests_total      |           N            |
|     hive_kube_client_request_seconds      |           N            |
| hive_kube_client_requests_cancelled_total |           N            |

#### ClusterDeployment controller metrics
These metrics are observed while processing ClusterDeployments. None of these are optional.

|                       Metric Name                        | Optional Label Support |
|:--------------------------------------------------------:|:----------------------:|
|   hive_cluster_deployment_install_job_duration_seconds   |           N            |
|    hive_cluster_deployment_install_job_delay_seconds     |           N            |
|    hive_cluster_deployment_imageset_job_delay_seconds    |           N            |
|        hive_cluster_deployment_dns_delay_seconds         |           N            |
|    hive_cluster_deployment_completed_install_restart     |           Y            |
|          hive_cluster_deployments_created_total          |           Y            |
|         hive_cluster_deployments_installed_total         |           Y            |
|          hive_cluster_deployments_deleted_total          |           Y            |
| hive_cluster_deployments_provision_failed_terminal_total |           Y            |

#### ClusterProvision controller metrics
These metrics are observed while processing ClusterProvisions. None of these are optional.

|                  Metric Name                  | Optional Label Support |
|:---------------------------------------------:|:----------------------:|
|     hive_cluster_provision_results_total      |           Y            |
|              hive_install_errors              |           Y            |
| hive_cluster_deployment_install_failure_total |           Y            |
| hive_cluster_deployment_install_success_total |           Y            |

#### ClusterPool controller metrics
These metrics are observed while processing ClusterPools. None of these are optional.

|                    Metric Name                    | Optional Label Support |
|:-------------------------------------------------:|:----------------------:|
|  hive_clusterpool_clusterdeployments_assignable   |           N            |
|    hive_clusterpool_clusterdeployments_claimed    |           N            |
|   hive_clusterpool_clusterdeployments_deleting    |           N            |
|  hive_clusterpool_clusterdeployments_installing   |           N            |
|   hive_clusterpool_clusterdeployments_unclaimed   |           N            |
|    hive_clusterpool_clusterdeployments_standby    |           N            |
|     hive_clusterpool_clusterdeployments_stale     |           N            |
|    hive_clusterpool_clusterdeployments_broken     |           N            |
| hive_clusterpool_stale_clusterdeployments_deleted |           N            |
|    hive_clusterclaim_assignment_delay_seconds     |           N            |

#### Metrics controller metrics
These metrics are accumulated across all instance of that type.
Some of these metrics are optional and the admin can opt for logging them via `HiveConfig.Spec.MetricsConfig.MetricsWithDuration`

|                          Metric Name                           | Optional Label Support | Optional |
|:--------------------------------------------------------------:|:----------------------:|:--------:|
|                    hive_cluster_deployments                    |           N            |    N     |
|               hive_cluster_deployments_installed               |           N            |    N     |
|              hive_cluster_deployments_uninstalled              |           N            |    N     |
|            hive_cluster_deployments_deprovisioning             |           N            |    N     |
|              hive_cluster_deployments_conditions               |           N            |    N     |
|                       hive_install_jobs                        |           N            |    N     |
|                      hive_uninstall_jobs                       |           N            |    N     |
|                       hive_imageset_jobs                       |           N            |    N     |
|              hive_selectorsyncset_clusters_total               |           N            |    N     |
|         hive_selectorsyncset_clusters_unapplied_total          |           N            |    N     |
|                      hive_syncsets_total                       |           N            |    N     |
|                 hive_syncsets_unapplied_total                  |           N            |    N     |
|      hive_cluster_deployment_deprovision_underway_seconds      |           N            |    N     |
|                hive_clustersync_failing_seconds                |           N            |    N     |
|    hive_cluster_deployments_hibernation_transition_seconds     |           N            |    Y     |
|      hive_cluster_deployments_running_transition_seconds       |           N            |    Y     |
|           hive_cluster_deployments_stopping_seconds            |           N            |    Y     |
|           hive_cluster_deployments_resuming_seconds            |           N            |    Y     |
| hive_cluster_deployments_waiting_for_cluster_operators_seconds |           N            |    Y     |
|               hive_controller_reconcile_seconds                |           N            |    N     |
|             hive_cluster_deployment_syncset_paused             |           N            |    N     |
|       hive_cluster_deployment_provision_underway_seconds       |           N            |    N     |
|  hive_cluster_deployment_provision_underway_install_restarts   |           N            |    N     |