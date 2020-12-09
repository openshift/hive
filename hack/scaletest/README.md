# Goal

Simulate 1000 clusters in Hive and observe performance. Because we do not have the budget or resources to actually provision 1000 clusters, Hive now supports a hive.openshift.io/fake-cluster=true annotation on the ClusterDeployment. When enabled, Hive will launch an install pod as normal, but skip the create-cluster stage and no resources should be provisioned in the cloud. It then reports fake data for admin kubeconfig, password, and various cluster status.

Once provisioned all communication with the cluster should be faked and the ClusterDeployment should never be marked Unreachable.

On deprovision we launch deprovision pods as usual, but with a fake infra ID there is nothing to delete and they should terminate quickly.

# Setup

Configure HiveConfig to use more than the default 5 goroutines per controller for the clustersync controller, responsible for applying SyncSets:

```yaml
apiVersion: hive.openshift.io/v1
kind: HiveConfig
spec:
  controllersConfig:
    controllers:
    - config:
        concurrentReconciles: 25
      name: clustersync
```

Create 60 SelectorSyncSets, each will contain 10 unique ConfigMaps with a little data, in the default namespace:

```bash
hack/scaletest/setup-selectorsyncsets.sh
```

Setup a prometheus pod with storage in the hive namespace. This will scrape Hive controllers and clustersync pods and allow us to monitor performance.


```bash
oc apply -f config/prometheus/prometheus-configmap.yaml
oc apply -f config/prometheus/prometheus-deployment-with-pvc.yaml
oc port-forward svc/prometheus -n hive 9091:9090
```

Create as many ClusterDeployments with the fake-cluster annotation as you like. This script can be re-run as needed, both over already created clusters, or to add more by adjusting the start/end indicies.

```
$ hack/scaletest/test_setup.sh 1 250
```

Load the [Prometheus WebUI](http://localhost:9091/graph).

Use [this link](http://localhost:9091/new/graph?g0.expr=workqueue_depth&g0.tab=0&g0.stacked=0&g0.range_input=2h&g1.expr=hive_syncsetinstance_apply_duration_seconds_sum%20%2F%20hive_syncsetinstance_apply_duration_seconds_count&g1.tab=0&g1.stacked=0&g1.range_input=1h&g2.expr=rate(hive_syncsetinstance_resources_applied_total%5B1m%5D)&g2.tab=0&g2.stacked=0&g2.range_input=1h&g3.expr=sum%20without(instance%2Cstatus%2Cresource)(hive_kube_client_request_seconds_sum%20%2F%20hive_kube_client_request_seconds_count%7Bremote%3D%22true%22%7D)&g3.tab=0&g3.stacked=0&g3.range_input=1h&g4.expr=rate(controller_runtime_reconcile_total%5B1m%5D)&g4.tab=0&g4.stacked=0&g4.range_input=15m&g5.expr=sum%20without(name)(hive_selectorsyncset_apply_duration_seconds_sum)%2Fsum%20without(name)(hive_selectorsyncset_apply_duration_seconds_count)&g5.tab=0&g5.stacked=0&g5.range_input=1h&g6.expr=sum%20without(instance%2Cstatus%2Cresource)(hive_kube_client_request_seconds_sum%7Bremote%3D%22false%22%7D%20%2F%20hive_kube_client_request_seconds_count%7Bremote%3D%22false%22%7D)&g6.tab=0&g6.stacked=0&g6.range_input=1h&g7.expr=rate(hive_kube_client_requests_total%5B5m%5D)&g7.tab=0&g7.stacked=0&g7.range_input=1h) for the graphs I was using for testing.

See the [Scaling Hive](../../docs/scaling-hive.md) documentation for recommendations resulting from this simulated scale testing.
