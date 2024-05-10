<table>
<tr>
<th>Title</th>
<th>Author</th>
<th colspan="7">Reviewers</th>
<th>Approver</th>
</tr>
<tr>
<td>HiveConfig.spec.metricsConfig redesign</td>
<td>@suhanime</td>
<td>@tzvatot</td>
<td>@bmeng</td>
<td>@dustman9000</td>
<td>@dofinn</td>
<td>@janboll</td>
<td>@berenss</td>
<td>@hongkailiu</td>
<td>@2uasimojo</td>
</tr>
</table>

# HiveConfig.spec.metricsConfig redesign

[HIVE-2344](https://issues.redhat.com/browse/HIVE-2344)

- [Summary](#summary)
- [Motivation](#motivation)
  - [Current implementations](#current-implementations)
    - [Duration based threshold](#duration-based-threshold)
    - [Additional label support](#additional-label-support)
  - [Goals](#goals)
  - [Non Goals](#non-goals)
- [Proposal](#proposal)
  - [metricsToReport](#metricstoreport)
    - [metricNames](#metricnames)
    - [minimumDuration](#minimumduration)
    - [additionalClusterDeploymentLabels](#additionalclusterdeploymentlabels)
    - [clusterDeploymentLabelSelector](#clusterdeploymentlabelselector)
  - [Implementation Details / Notes](#implementation-details--notes)
    - [Deprecation](#deprecation)
    - [Failure Modes](#failure-modes) 
  - [Risks and Mitigations](#risks-and-mitigations)

## Summary
As a Hive user interested in consuming the metrics it publishes, I would like to be able to configure the metrics as per my needs. 
This includes the ability to choose which metrics are published, reduce the amount of observations for certain metrics unless it meets configured conditions, and the ability to add labels to the reported metrics for grouping and filtering purposes.

## Motivation
Hive administrators tend to use metrics for monitoring, alerting and troubleshooting.

For monitoring and alerting, customers often want to apply their own filters/labels to the metrics, especially when 1 hive instance is used to manage different kinds of clusters, so they could tailor their queries and SLOs per their needs. For example, private-link clusters might take longer to install, or not wanting alerts to be thrown for clusters in limited support.
These labels or filters are unique to their use case, so instead of codifying them for relevant metrics, the changes brought on by this enhancement would allow the admins to configure them via their hiveConfig.

Troubleshooting involves identifying individual objects (clusters, syncsets, ...) via CRs and logs. In order to minimize the cognitive load of tracking down the offending object, there have been countless asks over the years to allow for applying object names as labels to the metrics.
Prometheus metrics are designed for aggregation - identifying individual contributors to a metric is a known difficult use case because every label on a metric results in a whole new time series in the database. Labels with unbounded cardinality -- like cluster ID -- can result in unbounded growth and eventual dysfunction of the Prom DB. Thus, it is generally not recommended to label metrics with unique identifiers (like cluster IDs).

This has been the major motivation for this enhancement. With the help of established baselines, admins can, via configuration, 
- Enumerate exactly which metrics are to be reported.
- Limit reporting metrics unless they exceed some defined boundary.
- Label metrics arbitrarily, including with unique identifiers such as cluster IDs.
- Employ LabelSelectors to group or limit clusters for which metrics are reported.

### Current implementations
Hive [publishes many metrics](../hive_metrics.md) that can be used for observations and alerting. Presently we offer 2 major customizations for some metrics:

#### Duration based threshold
Certain duration based metrics are used for alerting. While making them as specific as possible to quickly flag an issue with a cluster, it is also necessary to be mindful of the cardinality.
It is possible to configure a threshold duration, so that metric wouldn't be logged until the value to be reported matches or exceeds the configured duration.
It is important to call out these metrics are optional - hive doesn't report them by default.

- **Pros for this design**:
  - Threshold can be configured as per the needs of the alerts, so they're reported when needed, and there's less likelihood of these alerts needing to be silenced.
  - The customization has a 1-1 relationship with the metric itself.
  - These metrics happen to be optional, and not reported by default. 
- **Cons we would like to overcome**: Given how we have designed it, raw/whole metric name is not used in the hiveConfig - instead, a corresponding camelCase key is hardcoded to refer to this metric. We would want to move away from such design, and implement a different - less confusing strategy that can be expandable.

#### Additional label support
Hive can manage a lot of clusters, so we usually avoid labelling any metric with cluster name or namespace to keep the cardinality in check. So, in addition to the labels hive reports for a metric, some admins might want extra labels.
For example, if admins can identify that a metric corresponds to a "managed" cluster, they can apply effective filters for their observability and can observe trends across only managed clusters, and also customize their alerts.

- **Pros for this design**: The process of applying the labels to clusterDeployment, configuring those labels in hiveConfig to be reported with the metric, and using them as filters for alerts and grafana queries - is all in the hands of the admins/users and does not require any code change.
It also puts the onus of cardinality on the admins.
- **Cons we would like to overcome**: 
  - The way the code is written right now - we have a hardcoded hack for cluster-type label as mandatory if no additional labels are configured, and, most importantly, any additional labels configured will be applied to all the metrics which have optional label support, even though they might not be needed for all.
  We're essentially worsening the cardinality this way, and it makes adding this support to more metrics tricky. 
  - There's also the caveat of parsing through fixed and optional labels - ensuring there's no overlap and maintaining the order for certain metrics which do not accept map of label-value while observing, are some complications that currently exist and probably will not be fixed as a part of this enhancement.


### Goals
- Make all Hive metrics optional. By this design, no metric is to be reported by default unless it is explicitly configured in HiveConfig.
- Allow configuring additional labels, duration based threshold and expression matching support for all metrics that are related to cluster deployment, via HiveConfig.
- Enforce naming convention for duration based metrics - all metrics that report duration and allow customization for duration-based threshold must be named `*_seconds`.
- Move away from throwing a panic if metricsConfig fails validation, update HiveConfig.status.Conditions instead.

### Non-Goals
Here are some special call-outs pertaining to what will be out-of-scope for this proposal, however this proposal is designed to support future expansion, so we may work on these in the future.
- Non-clusterDeployment related metrics - like metrics reported by HiveOperator, or metrics that do not have the related clusterDeployment available while reporting (for ex, some syncSet, selectorSyncSet and clusterPool metrics) will not support additional label and expression matching. 
- Adding labels only refers to the labels propagated from the clusterDeployment, any other strings as labels cannot be configured this way.


## Proposal
Allow hiveConfig.Spec.metricsConfig to look like
```
hiveConfig:
  spec:
    metricsConfig:
      metricsToReport:
      - metricNames:
        - hive_foo_histogram
        - hive_foo_gauge
        minimumDuration: 10m
        clusterDeploymentLabelSelector:
          matchLabels:
            hive.openshift.io/aro-snowflake: "true"
          matchExpressions:
          - key: hive.openshift.io/limited-support
            operator: NotIn
            values: 
            - "true"
          - key: hive.openshift.io/limited-support
            operator: NotExists
        additionalClusterDeploymentLabels:
          prom_label_name: hive.openshift.io/cd-label-key
     - metricNames:
       - hive_foo_counter
```
### metricsToReport
We would now default to not reporting any metrics unless they have been explicitly listed out under `metricsToReport`
Within `metricsToReport`, there needs to be a non-empty list of hive metrics following an entry for `metricNames`, and there can be optional entries for customizations `minimumDuration`, `clusterDeploymentLabelSelector` and `additionalClusterDeploymentLabels`.
`metricsToReport` is a list, each element of which requests reporting and configures filtering and customizations for the metrics provided in its `metricNames` list.

#### metricNames
`metricNames` would be a non-empty list of hive metrics that need to be reported, and can have optional customizations.
All customizations listed for an entry of `metricToReport` will apply to all the metrics provided in that entry's `metricNames` list. This allows for grouping filters for a list of relevant metrics.
A metric name must appear at most once across all `metricsToReport[].metricNames[]` in order to avoid ambiguity.
Implementation will be adapting the existing setup for optional metrics. However, instead of a shorthand camelCase key that is used for current `hiveconfig.spec.metricsConfig.metricsWithDuration`, we would now need the full metric name listed under `metricNames`.
In the example above, hive would only log `hive_foo_counter`, `hive_foo_gauge` and `hive_foo_histogram` metrics.

#### minimumDuration
Deprecate the current implementation of hiveConfig.spec.metricsConfig.metricsWithDuration, and change it to be reported as metricsConfig.metricsToReport.metricNames[].minimumDuration. The implementation of using the duration as a threshold before we report the metric stays the same.
In the example above, `hive_foo_histogram` and `hive_foo_gauge` will only be logged if the value reported for them exceeds 10 minutes.

#### additionalClusterDeploymentLabels
Deprecate current implementation of hiveConfig.spec.metricsConfig.additionalClusterDeploymentLabels and change it to be reported as metricsConfig.metricsToReport.metricNames[].additionalClusterDeploymentLabels. Its implementation will not change.
In the example above, `hive_foo_histogram` and `hive_foo_gauge` would report an additional label `prom_label_name`, its value corresponding to the value of `hive.openshift.io/cd-label-key` label on the corresponding clusterDeployment.

#### clusterDeploymentLabelSelector
This would be a new feature, of type [LabelSelector](https://pkg.go.dev/k8s.io/apimachinery/pkg/apis/meta/v1#LabelSelector), and we'd use the LabelSelector.MatchLabels and/or LabelSelector.MatchExpressions to match the conditions in order to decide if a metric should be reported.
This encapsulates slightly more advanced filter logic over the existing clusterDeployment labels. In the example above, `hive_foo_histogram` and `hive_foo_gauge` metrics will only be reported for the clusterDeployments that are labelled with aro-snowflake and not in limited support.

### Implementation Details / Notes

- All the options configured for a `metricsToReport` entry will work in tandem with each other for each metric listed in its `metricNames`. For ex, if all possible options are specified for a metric, then that metric will be reported with the additional labels as per additionalClusterDeploymentLabels, and will be reported only if it matches the labels and/or expressions as per clusterDeploymentLabelSelector and if the duration to be reported exceeds the minimumDuration.
- This design allows for grouping of metrics for which you would want to define particular customizations. Grouping allows hiveConfig to be parsable. To avoid ambiguity, we will not allow multiple entries for any hive metric within `metricsToReport`.
- We would be removing the fixed labels that can be propagated from the clusterDeployment, i.e.`cluster_type`, `platform` and `version`. This means, if admins want these labels for the metrics, they will have to define them via  metricsConfig.metricsToReport[].additionalClusterDeploymentLabels. This would only be a breaking change if the admins suddenly cut over from deprecated/old way of metricsConfig to the new metricsToReport, sufficient documentation should help though.
- We would be renaming `hive_cluster_deployment_install_failure_total` and `hive_cluster_deployment_install_success_total` to ensure it ends with `seconds`, as it is a duration-based metric. Note that this will result in cloning the existing metric for the deprecation period, because if `metricsToReport` isn't present in the hiveConfig, then the _old_ way has to still work.
- We would no longer panic when the metricsConfig fails validation, instead we would be updating hiveConfig.Status.Conditions[ReadyCondition] to false, with appropriate reason and message. See [Failure Modes](#failure-modes) for more details.

#### Deprecation

In order to implement this change, we will have to deprecate the existing hiveConfig.spec.metricsConfig.metricsWithDuration and hiveConfig.spec.metricsConfig.additionalClusterDeploymentLabels.
As a way to handle the deprecation, we will assert that in a hiveConfig, you can either provide the new configurations (aka `metricsToReport`) or the old way of hiveConfig.spec.metricsConfig.metricsWithDuration and hiveConfig.spec.metricsConfig.additionalClusterDeploymentLabels. If there's an empty metricsConfig, we would default to _old_ behaviour for the period of deprecation.
Note that only the old design will support the mapping of camelCase key to metric for minimumDuration, the new way will require full metric names only. The old design will log `hive_cluster_deployment_install_failure_total` and `hive_cluster_deployment_install_success_total`, in the new design, they will be renamed to `hive_cluster_deployment_install_failure_total_seconds` and `hive_cluster_deployment_install_success_total_seconds`. 

#### Failure modes

These situations will result in hive-operator failing to deploy the controllers. It will set hiveConfig.Status.Conditions[ReadyCondition] to `"False"` with an appropriate error message.
-  `metricsToReport[].metricNames[$name]` doesn't exist as a metric.
- `metricNames` lists a metric without duration but `minimumDuration` is specified in the same `metricsToReport` entry.
- the same metric is mentioned more than once across all `metricsToReport[].metricNames[]`.
- additionalClusterDeploymentLabels key conflict with an existing fixed label key for that metric.
- if deprecated methods are used along with the new metricsToReport. You can either choose the _old_ way or the _new_ way.


### Risks and Mitigations
The biggest risk is the sheer number of metrics that are going to be affected. This would require thorough testing to ensure no metric changes its behaviour unexpectedly.

It can get confusing for consumers to know the fixed labels each metric has. Update the [hive_metrics](https://github.com/openshift/hive/blob/master/docs/hive_metrics.md) doc to list out all labels and keep it up-to-date. We may, in the future, attempt to autogenerate these docs -- see [Hive-2413](https://issues.redhat.com/browse/HIVE-2413).

Also, the more customizations there are, the longer hiveConfig will be. We have allowed for grouping of metrics for a set of customizations as a countermeasure.
