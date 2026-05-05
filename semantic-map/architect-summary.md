# Architect summary (machine-generated)

Deterministic digest of **`go-facts.json`**, **`repo-tree.json`**, and **`deps-graph.json`** for the Architect step. Regenerate with **`semantic-map architect-summary /Users/mworthin/GitHub/newtonheath/hive/semantic-map`** or **`analyze`**.

## Clone and module

- **Facts dir:** `/Users/mworthin/GitHub/newtonheath/hive`
- **Go module path:** `github.com/openshift/hive`

## Packages (128 from go-facts.json)

| Package path | Export count (`export_total`) | Exports truncated |
|--------------|------------------------------:|-------------------|
| `github.com/openshift/hive/cmd/hiveadmission` | 0 |  |
| `github.com/openshift/hive/cmd/manager` | 0 |  |
| `github.com/openshift/hive/cmd/operator` | 0 |  |
| `github.com/openshift/hive/cmd/util` | 1 |  |
| `github.com/openshift/hive/contrib/cmd/hiveutil` | 0 |  |
| `github.com/openshift/hive/contrib/cmd/waitforjob` | 0 |  |
| `github.com/openshift/hive/contrib/pkg/adm` | 1 |  |
| `github.com/openshift/hive/contrib/pkg/adm/managedns` | 6 |  |
| `github.com/openshift/hive/contrib/pkg/awsprivatelink` | 4 |  |
| `github.com/openshift/hive/contrib/pkg/awsprivatelink/common` | 2 |  |
| `github.com/openshift/hive/contrib/pkg/awsprivatelink/endpointvpc` | 2 |  |
| `github.com/openshift/hive/contrib/pkg/certificate` | 11 |  |
| `github.com/openshift/hive/contrib/pkg/clusterpool` | 5 |  |
| `github.com/openshift/hive/contrib/pkg/createcluster` | 6 |  |
| `github.com/openshift/hive/contrib/pkg/deprovision` | 9 |  |
| `github.com/openshift/hive/contrib/pkg/report` | 11 |  |
| `github.com/openshift/hive/contrib/pkg/testresource` | 1 |  |
| `github.com/openshift/hive/contrib/pkg/utils` | 12 |  |
| `github.com/openshift/hive/contrib/pkg/verification` | 3 |  |
| `github.com/openshift/hive/contrib/pkg/version` | 1 |  |
| `github.com/openshift/hive/hack` | 0 |  |
| `github.com/openshift/hive/pkg/awsclient` | 10 |  |
| `github.com/openshift/hive/pkg/awsclient/mock` | 110 |  |
| `github.com/openshift/hive/pkg/azureclient` | 4 |  |
| `github.com/openshift/hive/pkg/azureclient/mock` | 52 |  |
| `github.com/openshift/hive/pkg/clusterresource` | 46 |  |
| `github.com/openshift/hive/pkg/constants` | 171 |  |
| `github.com/openshift/hive/pkg/controller/argocdregister` | 8 |  |
| `github.com/openshift/hive/pkg/controller/awsprivatelink` | 6 |  |
| `github.com/openshift/hive/pkg/controller/clusterclaim` | 5 |  |
| `github.com/openshift/hive/pkg/controller/clusterdeployment` | 10 |  |
| `github.com/openshift/hive/pkg/controller/clusterdeprovision` | 5 |  |
| `github.com/openshift/hive/pkg/controller/clusterpool` | 5 |  |
| `github.com/openshift/hive/pkg/controller/clusterpoolnamespace` | 6 |  |
| `github.com/openshift/hive/pkg/controller/clusterprovision` | 4 |  |
| `github.com/openshift/hive/pkg/controller/clusterrelocate` | 4 |  |
| `github.com/openshift/hive/pkg/controller/clusterstate` | 6 |  |
| `github.com/openshift/hive/pkg/controller/clustersync` | 14 |  |
| `github.com/openshift/hive/pkg/controller/clusterversion` | 5 |  |
| `github.com/openshift/hive/pkg/controller/controlplanecerts` | 7 |  |
| `github.com/openshift/hive/pkg/controller/dnsendpoint` | 4 |  |
| `github.com/openshift/hive/pkg/controller/dnsendpoint/nameserver` | 1 |  |
| `github.com/openshift/hive/pkg/controller/dnsendpoint/nameserver/mock` | 9 |  |
| `github.com/openshift/hive/pkg/controller/dnszone` | 33 |  |
| `github.com/openshift/hive/pkg/controller/fakeclusterinstall` | 6 |  |
| `github.com/openshift/hive/pkg/controller/hibernation` | 7 |  |
| `github.com/openshift/hive/pkg/controller/hibernation/mock` | 29 |  |
| `github.com/openshift/hive/pkg/controller/images` | 6 |  |
| `github.com/openshift/hive/pkg/controller/machinepool` | 21 |  |
| `github.com/openshift/hive/pkg/controller/machinepool/mock` | 5 |  |
| `github.com/openshift/hive/pkg/controller/metrics` | 24 |  |
| `github.com/openshift/hive/pkg/controller/privatelink` | 9 |  |
| `github.com/openshift/hive/pkg/controller/privatelink/actuator` | 4 |  |
| `github.com/openshift/hive/pkg/controller/privatelink/actuator/awsactuator` | 8 |  |
| `github.com/openshift/hive/pkg/controller/privatelink/actuator/gcpactuator` | 5 |  |
| `github.com/openshift/hive/pkg/controller/privatelink/actuator/mock` | 11 |  |
| `github.com/openshift/hive/pkg/controller/privatelink/conditions` | 5 |  |
| `github.com/openshift/hive/pkg/controller/remoteingress` | 7 |  |
| `github.com/openshift/hive/pkg/controller/syncidentityprovider` | 7 |  |
| `github.com/openshift/hive/pkg/controller/unreachable` | 6 |  |
| `github.com/openshift/hive/pkg/controller/utils` | 137 |  |
| `github.com/openshift/hive/pkg/controller/utils/nutanixutils` | 3 |  |
| `github.com/openshift/hive/pkg/controller/utils/vsphereutils` | 1 |  |
| `github.com/openshift/hive/pkg/controller/velerobackup` | 6 |  |
| `github.com/openshift/hive/pkg/creds` | 1 |  |
| `github.com/openshift/hive/pkg/creds/aws` | 2 |  |
| `github.com/openshift/hive/pkg/creds/azure` | 2 |  |
| `github.com/openshift/hive/pkg/creds/gcp` | 2 |  |
| `github.com/openshift/hive/pkg/creds/ibmcloud` | 1 |  |
| `github.com/openshift/hive/pkg/creds/nutanix` | 1 |  |
| `github.com/openshift/hive/pkg/creds/openstack` | 2 |  |
| `github.com/openshift/hive/pkg/creds/vsphere` | 1 |  |
| `github.com/openshift/hive/pkg/gcpclient` | 11 |  |
| `github.com/openshift/hive/pkg/gcpclient/mock` | 67 |  |
| `github.com/openshift/hive/pkg/ibmclient` | 26 |  |
| `github.com/openshift/hive/pkg/ibmclient/mock` | 37 |  |
| `github.com/openshift/hive/pkg/imageset` | 8 |  |
| `github.com/openshift/hive/pkg/install` | 8 |  |
| `github.com/openshift/hive/pkg/installmanager` | 6 |  |
| `github.com/openshift/hive/pkg/manageddns` | 1 |  |
| `github.com/openshift/hive/pkg/operator` | 2 |  |
| `github.com/openshift/hive/pkg/operator/assets` | 7 |  |
| `github.com/openshift/hive/pkg/operator/hive` | 12 |  |
| `github.com/openshift/hive/pkg/operator/metrics` | 7 |  |
| `github.com/openshift/hive/pkg/remoteclient` | 6 |  |
| `github.com/openshift/hive/pkg/remoteclient/mock` | 15 |  |
| `github.com/openshift/hive/pkg/resource` | 7 |  |
| `github.com/openshift/hive/pkg/resource/mock` | 21 |  |
| `github.com/openshift/hive/pkg/test/assert` | 6 |  |
| `github.com/openshift/hive/pkg/test/checkpoint` | 3 |  |
| `github.com/openshift/hive/pkg/test/clusterclaim` | 3 |  |
| `github.com/openshift/hive/pkg/test/clusterdeployment` | 3 |  |
| `github.com/openshift/hive/pkg/test/clusterdeploymentcustomization` | 3 |  |
| `github.com/openshift/hive/pkg/test/clusterdeprovision` | 3 |  |
| `github.com/openshift/hive/pkg/test/clusterpool` | 3 |  |
| `github.com/openshift/hive/pkg/test/clusterprovision` | 3 |  |
| `github.com/openshift/hive/pkg/test/clusterrelocate` | 3 |  |
| `github.com/openshift/hive/pkg/test/clustersync` | 3 |  |
| `github.com/openshift/hive/pkg/test/configmap` | 3 |  |
| `github.com/openshift/hive/pkg/test/dnszone` | 3 |  |
| `github.com/openshift/hive/pkg/test/fake` | 6 |  |
| `github.com/openshift/hive/pkg/test/generic` | 1 |  |
| `github.com/openshift/hive/pkg/test/job` | 3 |  |
| `github.com/openshift/hive/pkg/test/logger` | 2 |  |
| `github.com/openshift/hive/pkg/test/machinepool` | 3 |  |
| `github.com/openshift/hive/pkg/test/manager` | 1 |  |
| `github.com/openshift/hive/pkg/test/manager/mock` | 39 |  |
| `github.com/openshift/hive/pkg/test/namespace` | 3 |  |
| `github.com/openshift/hive/pkg/test/secret` | 3 |  |
| `github.com/openshift/hive/pkg/test/selectorsyncset` | 3 |  |
| `github.com/openshift/hive/pkg/test/statefulset` | 3 |  |
| `github.com/openshift/hive/pkg/test/syncidentityprovider` | 3 |  |
| `github.com/openshift/hive/pkg/test/syncset` | 3 |  |
| `github.com/openshift/hive/pkg/util/contracts` | 6 |  |
| `github.com/openshift/hive/pkg/util/labels` | 1 |  |
| `github.com/openshift/hive/pkg/util/logrus` | 2 |  |
| `github.com/openshift/hive/pkg/util/scheme` | 1 |  |
| `github.com/openshift/hive/pkg/util/yaml` | 2 |  |
| `github.com/openshift/hive/pkg/validating-webhooks/hive/v1` | 36 |  |
| `github.com/openshift/hive/pkg/version` | 2 |  |
| `github.com/openshift/hive/test/e2e/common` | 22 |  |
| `github.com/openshift/hive/test/e2e/destroycluster` | 0 |  |
| `github.com/openshift/hive/test/e2e/postdeploy/admission` | 0 |  |
| `github.com/openshift/hive/test/e2e/postdeploy/hivecontroller` | 0 |  |
| `github.com/openshift/hive/test/e2e/postdeploy/operator` | 0 |  |
| `github.com/openshift/hive/test/e2e/postinstall/machinesets` | 0 |  |
| `github.com/openshift/hive/test/e2e/postinstall/syncsets` | 0 |  |
| `github.com/openshift/hive/test/e2e/uninstallhive` | 0 |  |

## Polyglot (shallow)

- **Bash/shell:** 25 file(s) scanned (truncated: false)
- **Python:** 4 file(s) scanned (truncated: false)

## Dependencies (deps-graph.json)

- **Classifier:** stdlib membership from `go list std` for the toolchain used during analyze (preferred when available)
- **Module path:** `github.com/openshift/hive`
- **Edge count:** 2133
- **Edges (external):** 1190
- **Edges (same_module):** 491
- **Edges (special):** 1
- **Edges (stdlib):** 451
- **Distinct external import paths:** 217

## Repository tree preview (repo-tree.json, depth ≤ 4)

*Clone:* `/Users/mworthin/GitHub/newtonheath/hive` · *depth_limit:* 8

- **.ai/** (`.ai`)
  - `README.md`
- **.tekton/** (`.tekton`)
  - `copyconfig.sh`
  - `hive-mce-210-pull-request.yaml`
  - `hive-mce-210-push.yaml`
  - `hive-mce-211-pull-request.yaml`
  - `hive-mce-211-push.yaml`
  - `hive-mce-217-pull-request.yaml`
  - `hive-mce-217-push.yaml`
  - `hive-mce-26-pull-request.yaml`
  - `hive-mce-26-push.yaml`
  - `hive-mce-27-pull-request.yaml`
  - `hive-mce-27-push.yaml`
  - `hive-mce-28-pull-request.yaml`
  - `hive-mce-28-push.yaml`
  - `hive-mce-29-pull-request.yaml`
  - `hive-mce-29-push.yaml`
  - `hive-mce-50-pull-request.yaml`
  - `hive-mce-50-push.yaml`
  - `hive-mce-51-pull-request.yaml`
  - `hive-mce-51-push.yaml`
  - `hive-pull-request.yaml`
  - `hive-push.yaml`
- **apis/** (`apis`)
  - **helpers/** (`apis/helpers`)
    - `namer.go`
    - `namer_test.go`
  - **hive/** (`apis/hive`)
    - **v1/** (`apis/hive/v1`)
      - **agent/** (`apis/hive/v1/agent`)
      - **aws/** (`apis/hive/v1/aws`)
      - **azure/** (`apis/hive/v1/azure`)
      - **baremetal/** (`apis/hive/v1/baremetal`)
      - **gcp/** (`apis/hive/v1/gcp`)
      - **ibmcloud/** (`apis/hive/v1/ibmcloud`)
      - **metricsconfig/** (`apis/hive/v1/metricsconfig`)
      - **none/** (`apis/hive/v1/none`)
      - **nutanix/** (`apis/hive/v1/nutanix`)
      - **openstack/** (`apis/hive/v1/openstack`)
      - **vsphere/** (`apis/hive/v1/vsphere`)
      - `checkpoint_types.go`
      - `clusterclaim_types.go`
      - `clusterdeployment_types.go`
      - `clusterdeploymentcustomization_types.go`
      - `clusterdeprovision_types.go`
      - `clusterimageset_types.go`
      - `clusterinstall_conditions.go`
      - `clusterpool_types.go`
      - `clusterprovision_types.go`
      - `clusterrelocate_types.go`
      - `clusterstate_types.go`
      - `conditions.go`
      - `dnszone_types.go`
      - `doc.go`
      - `hiveconfig_types.go`
      - `machinepool_types.go`
      - `machinepoolnamelease_types.go`
      - `metaruntimeobject.go`
      - `register.go`
      - `syncidentityprovider_types.go`
      - `syncset_types.go`
      - `zz_generated.deepcopy.go`
      - `zz_generated.defaults.go`
    - `group.go`
  - **hivecontracts/** (`apis/hivecontracts`)
    - **v1alpha1/** (`apis/hivecontracts/v1alpha1`)
      - `clusterinstall_types.go`
      - `doc.go`
      - `register.go`
      - `zz_generated.deepcopy.go`
    - `group.go`
  - **hiveinternal/** (`apis/hiveinternal`)
    - **v1alpha1/** (`apis/hiveinternal/v1alpha1`)
      - `clustersync_types.go`
      - `clustersynclease_types.go`
      - `doc.go`
      - `fakeclusterinstall_types.go`
      - `register.go`
      - `zz_generated.deepcopy.go`
    - `group.go`
  - **scheme/** (`apis/scheme`)
    - `scheme.go`
  - `addtoscheme_hive_v1.go`
  - `addtoscheme_hivecontracts_v1alpha1.go`
  - `addtoscheme_hiveinternal_v1alpha1.go`
  - `apis.go`
  - `go.mod`
  - `go.sum`
- **build/** (`build`)
  - **verify-imports/** (`build/verify-imports`)
    - `import-rules.yaml`
- **cmd/** (`cmd`)
  - **hiveadmission/** (`cmd/hiveadmission`)
    - `main.go`
  - **manager/** (`cmd/manager`)
    - `main.go`
  - **operator/** (`cmd/operator`)
    - `main.go`
  - **util/** (`cmd/util`)
    - `leaderelection.go`
- **config/** (`config`)
  - **buildconfig/** (`config/buildconfig`)
    - `hive-buildconfig.yaml`
    - `hive-imagestream.yaml`
    - `imagestream-rbac.yaml`
  - **configmaps/** (`config/configmaps`)
    - `install-log-regexes-configmap.yaml`
  - **controllers/** (`config/controllers`)
    - `deployment.yaml`
    - `hive_controllers_role.yaml`
    - `hive_controllers_role_binding.yaml`
    - `hive_controllers_serviceaccount.yaml`
    - `service.yaml`
  - **crds/** (`config/crds`)
    - `hive.openshift.io_checkpoints.yaml`
    - `hive.openshift.io_clusterclaims.yaml`
    - `hive.openshift.io_clusterdeploymentcustomizations.yaml`
    - `hive.openshift.io_clusterdeployments.yaml`
    - `hive.openshift.io_clusterdeprovisions.yaml`
    - `hive.openshift.io_clusterimagesets.yaml`
    - `hive.openshift.io_clusterpools.yaml`
    - `hive.openshift.io_clusterprovisions.yaml`
    - `hive.openshift.io_clusterrelocates.yaml`
    - `hive.openshift.io_clusterstates.yaml`
    - `hive.openshift.io_dnszones.yaml`
    - `hive.openshift.io_hiveconfigs.yaml`
    - `hive.openshift.io_machinepoolnameleases.yaml`
    - `hive.openshift.io_machinepools.yaml`
    - `hive.openshift.io_selectorsyncidentityproviders.yaml`
    - `hive.openshift.io_selectorsyncsets.yaml`
    - `hive.openshift.io_syncidentityproviders.yaml`
    - `hive.openshift.io_syncsets.yaml`
    - `hiveinternal.openshift.io_clustersyncleases.yaml`
    - `hiveinternal.openshift.io_clustersyncs.yaml`
    - `hiveinternal.openshift.io_fakeclusterinstalls.yaml`
  - **crdspatch/** (`config/crdspatch`)
    - `hive.openshift.io_clusterprovisions.yaml`
    - `hiveinternal.openshift.io_fakeclusterinstalls.yaml`
  - **hiveadmission/** (`config/hiveadmission`)
    - `apiservice.yaml`
    - `clusterdeployment-webhook.yaml`
    - `clusterdeploymentcustomization-webhook.yaml`
    - `clusterimageset-webhook.yaml`
    - `clusterpool-webhook.yaml`
    - `clusterprovision-webhook.yaml`
    - `deployment.yaml`
    - `dnszones-webhook.yaml`
    - `hiveadmission_rbac_role.yaml`
    - `hiveadmission_rbac_role_binding.yaml`
    - `machinepool-webhook.yaml`
    - `sa-token-secret.yaml`
    - `selectorsyncset-webhook.yaml`
    - `service-account.yaml`
    - `service.yaml`
    - `syncset-webhook.yaml`
  - **operator/** (`config/operator`)
    - `01_clusterimageset_crd.yaml`
    - `operator_deployment.yaml`
    - `operator_role.yaml`
    - `operator_role_binding.yaml`
  - **prometheus/** (`config/prometheus`)
    - `prometheus-configmap.yaml`
    - `prometheus-deployment-with-pvc.yaml`
    - `prometheus-deployment.yaml`
  - **rbac/** (`config/rbac`)
    - `hive_admin_role.yaml`
    - `hive_admin_role_binding.yaml`
    - `hive_clusterpool_admin.yaml`
    - `hive_frontend_role.yaml`
    - `hive_frontend_role_binding.yaml`
    - `hive_frontend_serviceaccount.yaml`
    - `hive_reader_role.yaml`
    - `hive_reader_role_binding.yaml`
  - **samples/** (`config/samples`)
    - **manifests/** (`config/samples/manifests`)
      - `sample-manifest.yaml`
    - `hive_v1_clusterdeployment.yaml`
    - `hive_v1_clusterdeprovision.yaml`
    - `hive_v1_clusterimageset.yaml`
    - `hive_v1_dnszone.yaml`
    - `hive_v1_machinepool.yaml`
    - `hive_v1_syncsetinstance.yaml`
  - **sharded_controllers/** (`config/sharded_controllers`)
    - `service.yaml`
    - `statefulset.yaml`
  - **templates/** (`config/templates`)
    - `cluster-deployment-customimageset.yaml`
    - `cluster-deployment.yaml`
    - `hive-csv-template.yaml`
    - `hiveconfig.yaml`
  - `kustomization.yaml`
  - `namespace.yaml`
- **contrib/** (`contrib`)
  - **cmd/** (`contrib/cmd`)
    - **hiveutil/** (`contrib/cmd/hiveutil`)
      - `main.go`
    - **waitforjob/** (`contrib/cmd/waitforjob`)
      - `main.go`
  - **pkg/** (`contrib/pkg`)
    - **adm/** (`contrib/pkg/adm`)
      - **managedns/** (`contrib/pkg/adm/managedns`)
      - `adm.go`
    - **awsprivatelink/** (`contrib/pkg/awsprivatelink`)
      - **common/** (`contrib/pkg/awsprivatelink/common`)
      - **endpointvpc/** (`contrib/pkg/awsprivatelink/endpointvpc`)
      - `awsprivatelink.go`
      - `disable.go`
      - `enable.go`
      - `endpointVPC.go`
    - **certificate/** (`contrib/pkg/certificate`)
      - `command.go`
      - `create.go`
      - `hook.go`
    - **clusterpool/** (`contrib/pkg/clusterpool`)
      - `clusterclaim.go`
      - `clusterpool.go`
      - `command.go`
    - **createcluster/** (`contrib/pkg/createcluster`)
      - `create.go`
      - `nutanix.go`
    - **deprovision/** (`contrib/pkg/deprovision`)
      - `awstagdeprovision.go`
      - `azure.go`
      - `deprovision.go`
      - `gcp.go`
      - `ibmcloud.go`
      - `nutanix.go`
      - `openstack.go`
      - `vsphere.go`
    - **report/** (`contrib/pkg/report`)
      - `deprovisioning.go`
      - `provisioning.go`
      - `report.go`
    - **testresource/** (`contrib/pkg/testresource`)
      - `command.go`
    - **utils/** (`contrib/pkg/utils`)
      - `client.go`
      - `generic.go`
    - **verification/** (`contrib/pkg/verification`)
      - `imports.go`
    - **version/** (`contrib/pkg/version`)
      - `version.go`
- **docs/** (`docs`)
  - **enhancements/** (`docs/enhancements`)
    - `aws-private-link-arch.png`
    - `aws-private-link.md`
    - `cluster-install-apis.md`
    - `cluster_pool_hot_spares.md`
    - `clusterpool-inventory.md`
    - `hivev2.md`
    - `install_logs.md`
    - `metricsConfig_redesign.md`
    - `patch-install-manifests.md`
    - `replace_broken_clusters.md`
    - `scale-mode.md`
  - `annotations.md`
  - `apiserver.md`
  - `architecture.md`
  - `aws-sts-provisioning.md`
  - `awsassumerolecreds.md`
  - `awsprivatelink.md`
  - `buildconfig.md`
  - `cluster-relocation.md`
  - `clusterpools.md`
  - `developing.md`
  - `FAQs.md`
  - `hibernating-clusters.md`
  - `hibernation_controller.png`
  - `Hive Logo final 329x329.png`
  - `hive-architecture.drawio`
  - `hive-architecture.png`
  - `hive-baremetal-hypervisor-per-cluster.png`
  - `hive-baremetal-shared-hypervisor.png`
  - `hive_metrics.md`
  - `hiveutil.md`
  - `install.md`
  - `managed-dns.md`
  - `microsoft_entra_workload_id.md`
  - `monitoring.md`
  - `move_clusters.md`
  - `privatelink.md`
  - `quick_start.md`
  - `releaseimageverify.md`
  - `scaling-hive.md`
  - `syncidentityprovider.md`
  - `syncset.md`
  - `syncset_apply_times_graph.png`
  - `troubleshooting.md`
  - `using-hive.md`
- **hack/** (`hack`)
  - **app-sre/** (`hack/app-sre`)
    - `generate-saas-template.sh`
    - `kustomization.yaml`
    - `saas-template-stub.yaml`
    - `saas-template.yaml`
  - **awsprivatelink/** (`hack/awsprivatelink`)
    - `vpc.cf.yaml`
  - **gcpprivateserviceconnect/** (`hack/gcpprivateserviceconnect`)
    - `linkvpc.py`
  - **grafana-dashboards/** (`hack/grafana-dashboards`)
    - `grafana-dashboard-hive-logs.configmap.yaml`
    - `grafana-dashboard-hive-slo.configmap.yaml`
  - **hermetic/** (`hack/hermetic`)
    - `README.md`
    - `redhat.repo`
    - `rpms.in.yaml`
    - `rpms.lock.yaml`
  - **regexes/** (`hack/regexes`)
    - `additional-hive-regexes.yaml`
  - **scaletest/** (`hack/scaletest`)
    - `README.md`
    - `setup-selectorsyncsets.sh`
    - `syncset-template.yaml`
    - `test_setup.sh`
  - `app_sre_build_deploy.sh`
  - `boilerplate.go.txt`
  - `bundle-gen.py`
  - `codecov.sh`
  - `create-kind-cluster.sh`
  - `create-service-account-secrets.sh`
  - `duplicate_cd.sh`
  - `e2e-common.sh`
  - `e2e-pool-test.sh`
  - `e2e-test.sh`
  - `fakeclusterinstall.yaml`
  - `get-kubeconfig.sh`
  - `github.py`
  - `hiveadmission-dev-cert.sh`
  - `local-e2e-test.sh`
  - `logextractor.sh`
  - `make`
  - `modcheck.go`
  - `refresh-clusterpool-creds.sh`
  - `requirements.txt`
  - `run-hive-locally.sh`
  - `set-additional-ca.sh`
  - `statuspatch`
  - `ubi-build-deps.sh`
  - `update-codegen.sh`
  - `verify-crd.sh`
  - `version2.sh`
- **overlays/** (`overlays`)
  - **template/** (`overlays/template`)
    - `kustomization.yaml`
  - `.gitignore`
- **pkg/** (`pkg`)
  - **awsclient/** (`pkg/awsclient`)
    - **mock/** (`pkg/awsclient/mock`)
      - `client_generated.go`
    - `client.go`
  - **azureclient/** (`pkg/azureclient`)
    - **mock/** (`pkg/azureclient/mock`)
      - `client_generated.go`
    - `client.go`
  - **clusterresource/** (`pkg/clusterresource`)
    - `aws.go`
    - `azure.go`
    - `builder.go`
    - `builder_test.go`
    - `gcp.go`
    - `ibmcloud.go`
    - `installconfigtemplate.go`
    - `installconfigtemplate_test.go`
    - `nutanix.go`
    - `openstack.go`
    - `vsphere.go`
  - **constants/** (`pkg/constants`)
    - `constants.go`
    - `constants_test.go`
    - `nutanix.go`
  - **controller/** (`pkg/controller`)
    - **argocdregister/** (`pkg/controller/argocdregister`)
      - `argocdregister_controller.go`
      - `argocdregister_controller_test.go`
      - `argotypes.go`
    - **awsprivatelink/** (`pkg/controller/awsprivatelink`)
      - `awsprivatelink_controller.go`
      - `awsprivatelink_controller_test.go`
      - `cleanup.go`
      - `cleanup_test.go`
      - `vpcinventory.go`
    - **clusterclaim/** (`pkg/controller/clusterclaim`)
      - `clusterclaim_controller.go`
      - `clusterclaim_controller_test.go`
    - **clusterdeployment/** (`pkg/controller/clusterdeployment`)
      - `clusterdeployment_controller.go`
      - `clusterdeployment_controller_test.go`
      - `clusterinstalls.go`
      - `clusterprovisions.go`
      - `installconfigvalidation.go`
      - `installconfigvalidation_test.go`
      - `metrics.go`
    - **clusterdeprovision/** (`pkg/controller/clusterdeprovision`)
      - `actuator.go`
      - `awsactuator.go`
      - `clusterdeprovision_controller.go`
      - `clusterdeprovision_controller_test.go`
      - `helpers_test.go`
    - **clusterpool/** (`pkg/controller/clusterpool`)
      - `clusterdeploymentexpectations.go`
      - `clusterpool_controller.go`
      - `clusterpool_controller_test.go`
      - `collections.go`
      - `metrics.go`
    - **clusterpoolnamespace/** (`pkg/controller/clusterpoolnamespace`)
      - `clusterpoolnamespace_controller.go`
      - `clusterpoolnamespace_controller_test.go`
    - **clusterprovision/** (`pkg/controller/clusterprovision`)
      - `clusterprovision_controller.go`
      - `clusterprovision_controller_test.go`
      - `installlogmonitor.go`
      - `installlogmonitor_test.go`
      - `installlogregex.go`
      - `jobexpectations.go`
      - `metrics.go`
    - **clusterrelocate/** (`pkg/controller/clusterrelocate`)
      - `clientwrapper_test.go`
      - `clusterrelocate_controller.go`
      - `clusterrelocate_controller_test.go`
    - **clusterstate/** (`pkg/controller/clusterstate`)
      - `clusterstate_controller.go`
      - `clusterstate_controller_test.go`
    - **clustersync/** (`pkg/controller/clustersync`)
      - `clientwrapper_test.go`
      - `clustersync_controller.go`
      - `clustersync_controller_test.go`
      - `commonsyncset.go`
      - `templates.go`
    - **clusterversion/** (`pkg/controller/clusterversion`)
      - `clusterversion_controller.go`
      - `clusterversion_controller_test.go`
    - **controlplanecerts/** (`pkg/controller/controlplanecerts`)
      - `controlplanecerts_controller.go`
      - `controlplanecerts_controller_test.go`
    - **dnsendpoint/** (`pkg/controller/dnsendpoint`)
      - **nameserver/** (`pkg/controller/dnsendpoint/nameserver`)
      - `dnsendpoint_controller.go`
      - `dnsendpoint_controller_test.go`
      - `nameserverscraper.go`
      - `nameserverscraper_test.go`
    - **dnszone/** (`pkg/controller/dnszone`)
      - `actuator.go`
      - `awsactuator.go`
      - `awsactuator_test.go`
      - `azureactuator.go`
      - `azureactuator_test.go`
      - `dnszone_controller.go`
      - `dnszone_controller_test.go`
      - `gcpactuator.go`
      - `gcpactuator_test.go`
      - `test_helpers.go`
    - **fakeclusterinstall/** (`pkg/controller/fakeclusterinstall`)
      - `fakeclusterinstall_controller.go`
    - **hibernation/** (`pkg/controller/hibernation`)
      - **mock/** (`pkg/controller/hibernation/mock`)
      - `aws_actuator.go`
      - `aws_actuator_test.go`
      - `azure_actuator.go`
      - `azure_actuator_test.go`
      - `csr_helper.go`
      - `csr_utility.go`
      - `gcp_actuator.go`
      - `gcp_actuator_test.go`
      - `hibernation_actuator.go`
      - `hibernation_controller.go`
      - `hibernation_controller_test.go`
      - `ibmcloud_actuator.go`
      - `ibmcloud_actuator_test.go`
    - **images/** (`pkg/controller/images`)
      - `controller_images.go`
    - **machinepool/** (`pkg/controller/machinepool`)
      - **mock/** (`pkg/controller/machinepool/mock`)
      - `actuator.go`
      - `awsactuator.go`
      - `awsactuator_test.go`
      - `azureactuator.go`
      - `azureactuator_test.go`
      - `constants.go`
      - `gcpactuator.go`
      - `gcpactuator_test.go`
      - `ibmcloudactuator.go`
      - `ibmcloudactuator_test.go`
      - `leaseexceptions.go`
      - `machinepool_controller.go`
      - `machinepool_controller_test.go`
      - `nutanixactuator.go`
      - `nutanixactuator_test.go`
      - `openstackactuator.go`
      - `openstackactuator_test.go`
      - `vsphereactuator.go`
      - `vsphereactuator_test.go`
    - **metrics/** (`pkg/controller/metrics`)
      - `custom_collectors.go`
      - `custom_collectors_test.go`
      - `metrics.go`
      - `metrics_test.go`
      - `metrics_with_dynamic_labels.go`
    - **privatelink/** (`pkg/controller/privatelink`)
      - **actuator/** (`pkg/controller/privatelink/actuator`)
      - **conditions/** (`pkg/controller/privatelink/conditions`)
      - `privatelink.go`
      - `privatelink_controller.go`
      - `privatelink_test.go`
    - **remoteingress/** (`pkg/controller/remoteingress`)
      - `remoteingress_controller.go`
      - `remoteingress_controller_test.go`
    - **syncidentityprovider/** (`pkg/controller/syncidentityprovider`)
      - `syncidentityprovider_controller.go`
      - `syncidentityprovider_controller_test.go`
    - **unreachable/** (`pkg/controller/unreachable`)
      - `unreachable_controller.go`
      - `unreachable_controller_test.go`
    - **utils/** (`pkg/controller/utils`)
      - **nutanixutils/** (`pkg/controller/utils/nutanixutils`)
      - **vsphereutils/** (`pkg/controller/utils/vsphereutils`)
      - `cacrt.go`
      - `clientwrapper.go`
      - `clientwrapper_test.go`
      - `clusterdeployment.go`
      - `clusterdeployment_test.go`
      - `clusterpool.go`
      - `conditions.go`
      - `conditions_test.go`
      - `credentials.go`
      - `delayingreconciler.go`
      - `dnszone.go`
      - `dnszone_test.go`
      - `domain.go`
      - `duck_types.go`
      - `errorscrub.go`
      - `errorscrub_test.go`
      - `expectations.go`
      - `expectations_test.go`
      - `jobs.go`
      - `logtagger.go`
      - `logtagger_test.go`
      - `ownership.go`
      - `ownership_test.go`
      - `podconfig.go`
      - `ratelimitedeventhandler.go`
      - `ratelimitedeventhandler_test.go`
      - `sa.go`
      - `sa_test.go`
      - `secrets.go`
      - `statefulset.go`
      - `statefulset_test.go`
      - `taints.go`
      - `utils.go`
      - `utils_test.go`
      - `watcher_inject.go`
    - **velerobackup/** (`pkg/controller/velerobackup`)
      - `helpers_test.go`
      - `velerobackup_controller.go`
      - `velerobackup_controller_test.go`
  - **creds/** (`pkg/creds`)
    - **aws/** (`pkg/creds/aws`)
      - `aws.go`
    - **azure/** (`pkg/creds/azure`)
      - `azure.go`
    - **gcp/** (`pkg/creds/gcp`)
      - `gcp.go`
    - **ibmcloud/** (`pkg/creds/ibmcloud`)
      - `ibmcloud.go`
    - **nutanix/** (`pkg/creds/nutanix`)
      - `nutanix.go`
    - **openstack/** (`pkg/creds/openstack`)
      - `openstack.go`
    - **vsphere/** (`pkg/creds/vsphere`)
      - `vsphere.go`
    - `creds.go`
  - **dependencymagnet/** (`pkg/dependencymagnet`)
    - `doc.go`
  - **gcpclient/** (`pkg/gcpclient`)
    - **mock/** (`pkg/gcpclient/mock`)
      - `client_generated.go`
    - `client.go`
  - **ibmclient/** (`pkg/ibmclient`)
    - **mock/** (`pkg/ibmclient/mock`)
      - `client_generated.go`
    - `client.go`
  - **imageset/** (`pkg/imageset`)
    - `generate.go`
    - `generate_test.go`
    - `updateinstaller.go`
    - `updateinstaller_test.go`
  - **install/** (`pkg/install`)
    - `generate.go`
    - `generate_test.go`
  - **installmanager/** (`pkg/installmanager`)
    - **testdata/** (`pkg/installmanager/testdata`)
      - `install-config-with-existing-pull-secret.yaml`
      - `install-config-with-pull-secret.yaml`
      - `install-config.yaml`
      - `nutanix-install-config-with-credentials.yaml`
      - `nutanix-install-config.yaml`
      - `pull-secret.json`
    - `dnscleanup.go`
    - `fake.go`
    - `helper_test.go`
    - `ibm_metadata_test.go`
    - `installmanager.go`
    - `installmanager_test.go`
    - `loguploaderactuator.go`
    - `s3loguploaderactuator.go`
    - `s3loguploaderactuator_test.go`
  - **manageddns/** (`pkg/manageddns`)
    - `manageddns.go`
  - **operator/** (`pkg/operator`)
    - **assets/** (`pkg/operator/assets`)
      - `bindata.go`
    - **hive/** (`pkg/operator/hive`)
      - `apply.go`
      - `conditions.go`
      - `configmap.go`
      - `dynamicclient.go`
      - `hive.go`
      - `hive_controller.go`
      - `hiveadmission.go`
      - `operatorutils.go`
      - `sharded_controllers.go`
    - **metrics/** (`pkg/operator/metrics`)
      - `metrics.go`
    - `add_hive.go`
    - `controller.go`
  - **remoteclient/** (`pkg/remoteclient`)
    - **mock/** (`pkg/remoteclient/mock`)
      - `remoteclient_generated.go`
    - **testdata/** (`pkg/remoteclient/testdata`)
      - `kubeconfig.sample`
    - `dialer.go`
    - `dialer_test.go`
    - `fake.go`
    - `kubeconfig.go`
    - `remoteclient.go`
    - `remoteclient_test.go`
  - **resource/** (`pkg/resource`)
    - **mock/** (`pkg/resource/mock`)
      - `helper_generated.go`
    - `apply.go`
    - `client.go`
    - `delete.go`
    - `factory_discovery.go`
    - `fake.go`
    - `helper.go`
    - `info.go`
    - `kubeconfig_factory.go`
    - `patch.go`
    - `patch_test.go`
    - `restconfig_factory.go`
    - `serializer.go`
  - **test/** (`pkg/test`)
    - **assert/** (`pkg/test/assert`)
      - `assertions.go`
    - **checkpoint/** (`pkg/test/checkpoint`)
      - `checkpoint.go`
    - **clusterclaim/** (`pkg/test/clusterclaim`)
      - `clusterclaim.go`
    - **clusterdeployment/** (`pkg/test/clusterdeployment`)
      - `clusterdeployment.go`
    - **clusterdeploymentcustomization/** (`pkg/test/clusterdeploymentcustomization`)
      - `clusterdeploymentcustomization.go`
    - **clusterdeprovision/** (`pkg/test/clusterdeprovision`)
      - `clusterdeprovision.go`
    - **clusterpool/** (`pkg/test/clusterpool`)
      - `clusterpool.go`
    - **clusterprovision/** (`pkg/test/clusterprovision`)
      - `clusterprovision.go`
    - **clusterrelocate/** (`pkg/test/clusterrelocate`)
      - `clusterrelocate.go`
    - **clustersync/** (`pkg/test/clustersync`)
      - `clustersync.go`
    - **configmap/** (`pkg/test/configmap`)
      - `configmap.go`
    - **dnszone/** (`pkg/test/dnszone`)
      - `dnszone.go`
    - **fake/** (`pkg/test/fake`)
      - `fake.go`
      - `overrideable_client.go`
    - **generic/** (`pkg/test/generic`)
      - `generic.go`
    - **job/** (`pkg/test/job`)
      - `job.go`
    - **logger/** (`pkg/test/logger`)
      - `logger.go`
    - **machinepool/** (`pkg/test/machinepool`)
      - `machinepool.go`
    - **manager/** (`pkg/test/manager`)
      - **mock/** (`pkg/test/manager/mock`)
      - `manager.go`
    - **namespace/** (`pkg/test/namespace`)
      - `namespace.go`
    - **secret/** (`pkg/test/secret`)
      - `secret.go`
    - **selectorsyncset/** (`pkg/test/selectorsyncset`)
      - `selectorsyncset.go`
    - **statefulset/** (`pkg/test/statefulset`)
      - `statefulset.go`
    - **syncidentityprovider/** (`pkg/test/syncidentityprovider`)
      - `syncidentityprovider.go`
    - **syncset/** (`pkg/test/syncset`)
      - `syncset.go`
  - **util/** (`pkg/util`)
    - **contracts/** (`pkg/util/contracts`)
      - `contracts.go`
      - `contracts_test.go`
    - **labels/** (`pkg/util/labels`)
      - `labels.go`
    - **logrus/** (`pkg/util/logrus`)
      - `eventrecorder.go`
      - `logr.go`
      - `logr_test.go`
    - **scheme/** (`pkg/util/scheme`)
      - `scheme.go`
    - **yaml/** (`pkg/util/yaml`)
      - `yaml.go`
  - **validating-webhooks/** (`pkg/validating-webhooks`)
    - **hive/** (`pkg/validating-webhooks/hive`)
      - **v1/** (`pkg/validating-webhooks/hive/v1`)
  - **version/** (`pkg/version`)
    - `version.go`
  - `OWNERS`
- **semantic-map/** (`semantic-map`)
  - **docs/** (`semantic-map/docs`)
    - **context/** (`semantic-map/docs/context`)
      - **cmd/** (`semantic-map/docs/context/cmd`)
      - **contrib/** (`semantic-map/docs/context/contrib`)
      - **hack/** (`semantic-map/docs/context/hack`)
      - **pkg/** (`semantic-map/docs/context/pkg`)
      - **test/** (`semantic-map/docs/context/test`)
      - `root.md`
  - `go-facts.json`
- **test/** (`test`)
  - **e2e/** (`test/e2e`)
    - **common/** (`test/e2e/common`)
      - `apiservice.go`
      - `client.go`
      - `clusterdeployment.go`
      - `deployment.go`
      - `diff.go`
      - `machine.go`
      - `machinepool.go`
      - `machineset.go`
      - `node.go`
      - `service.go`
      - `utils.go`
    - **destroycluster/** (`test/e2e/destroycluster`)
      - `destroy_test.go`
    - **postdeploy/** (`test/e2e/postdeploy`)
      - **admission/** (`test/e2e/postdeploy/admission`)
      - **hivecontroller/** (`test/e2e/postdeploy/hivecontroller`)
      - **operator/** (`test/e2e/postdeploy/operator`)
    - **postinstall/** (`test/e2e/postinstall`)
      - **machinesets/** (`test/e2e/postinstall/machinesets`)
      - **syncsets/** (`test/e2e/postinstall/syncsets`)
    - **uninstallhive/** (`test/e2e/uninstallhive`)
      - `uninstallhive_test.go`
  - **ote/** (`test/ote`)
    - **cmd/** (`test/ote/cmd`)
      - **extension/** (`test/ote/cmd/extension`)
    - **hive/** (`test/ote/hive`)
      - **testdata/** (`test/ote/hive/testdata`)
      - `fixtures.go`
      - `hive.go`
      - `hive_aws.go`
      - `hive_azure.go`
      - `hive_gcp.go`
      - `hive_util.go`
      - `hive_vsphere.go`
      - `suite_setup.go`
    - `bindata.mk`
    - `go.mod`
    - `go.sum`
    - `Makefile`
- `.codecov.yml`
- `.coderabbit.yaml`
- `.gitignore`
- `.snyk`
- `AGENTS.md`
- `CLAUDE.md`
- `CONTRIBUTING.md`
- `Dockerfile`
- `Dockerfile.ote`
- `go.mod`
- `go.sum`
- `golangci.yml`
- `LICENSE`
- `Makefile`
- `OWNERS`
- `PROJECT`
- `README.md`
- `renovate.json`

---
*Schema: semantic-map/architect-summary-v1 (Markdown prose)*
