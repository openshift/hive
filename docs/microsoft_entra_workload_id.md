# Microsoft Entra Workload ID Clusters Installation Procedures
It is possible to use Hive to provision clusters configured to use Microsoft Entra Workload Identity. This authentication method enables OpenShift clusters to use short-lived tokens.

## Table of Contents

- [Summary](#summary)
- [Procedure 1: Microsoft Entra Workload ID Cluster Installation](#procedure-1-microsoft-entra-workload-id-cluster-installation)
  - [Step 1: Extract the ccoctl binary from the release image](#step-1-extract-the-ccoctl-binary-from-the-release-image)
  - [Step 2: Creating Azure resources with the ccoctl tool](#step-2-creating-azure-resources-with-the-ccoctl-tool)
  - [Step 3: Verify Azure Resource Groups](#step-3-verify-azure-resource-groups)
  - [Step 4: Create Secret for Service Account Signing Key](#step-4-create-secret-for-service-account-signing-key)
  - [Step 5: Create Secret for Installer Manifests](#step-5-create-secret-for-installer-manifests)
  - [Step 6: Deploy Cluster with Hive](#step-6-deploy-cluster-with-hive)
    - [6.1 Install the Hive Operator](#61-install-the-hive-operator)
    - [6.2 Create Pull Secret](#62-create-pull-secret)
    - [6.3 Create ClusterImageSet](#63-create-clusterimageset)
    - [6.4 Create Azure Cloud Credentials Secret](#64-create-azure-cloud-credentials-secret)
    - [6.5 Create InstallConfig Secret](#65-create-installconfig-secret)
    - [6.6 Deploy ClusterDeployment](#66-deploy-clusterdeployment)
  - [Step 7: Verify Cluster Installation](#step-7-verify-cluster-installation)
  - [Step 8: Cleanup Azure resources after uninstalling the cluster](#step-8-cleanup-azure-resources-after-uninstalling-the-cluster)
- [Procedure 2: Microsoft Entra Workload ID Cluster Installation using hiveutil](#procedure-2-microsoft-entra-workload-id-cluster-installation-using-hiveutil)
  - [Steps 1-5: Same as Procedure 1](#steps-1-5-same-as-procedure-1-microsoft-entra-workload-id-cluster-installation)
  - [Step 6: Deploy Cluster using hiveutil](#step-6-deploy-cluster-using-hiveutil)
    - [6.1 Extract hiveutil binary](#61-extract-hiveutil-binary)
    - [6.2 Generate manifests using hiveutil](#62-generate-manifests-using-hiveutil)
    - [6.3 Update ClusterDeployment and InstallConfig](#63-update-clusterdeployment-and-installconfig)
    - [6.4 Deploy Cluster](#64-deploy-cluster)
  - [Step 7-8: Same as Procedure 1](#step-7-8-same-as-procedure-1-microsoft-entra-workload-id-cluster-installation)

## Summary

When installing a Microsoft Entra Workload ID cluster using Hive, the following specific configurations for the ClusterDeployment and InstallConfig resources are required:

- In your InstallConfig set `credentialsMode: Manual`
- In your InstallConfig, set `platform.azure.resourceGroupName` to the installation resource group created by `ccoctl azure create-all` 
- In your ClusterDeployment set `spec.boundServiceAccountSigningKeySecretRef.name` to point to the Secret created above (`bound-service-account-signing-key`) 
- In your ClusterDeployment set `spec.provisioning.manifestsSecretRef` to point to the Secret created above (`cluster-manifests`)

### Supported Hub-Spoke Configurations

- Hub running on public AWS → Spoke - Microsoft Entra Workload ID cluster (public)
- Hub running on public Azure → Spoke - Microsoft Entra Workload ID cluster (public)
- Hub running on public GCP → Spoke - Microsoft Entra Workload ID cluster (public)

## Procedure 1: Microsoft Entra Workload ID Cluster Installation

### Step 1: Extract the ccoctl binary from the release image

**Note:** The ccoctl utility is a Linux binary that must run in a Linux environment.

1. Extract the openshift-install and ccoctl binaries from the release image.
```bash
$ oc adm release extract --command=openshift-install $RELEASE_IMAGE
$ CCO_IMAGE=$(oc adm release info --image-for='cloud-credential-operator' ${RELEASE_IMAGE}) 
$ oc image extract ${CCO_IMAGE} --file='/usr/bin/ccoctl' --registry-config=${PULL_SECRET_PATH:-.}/pull-secret
```

2. Make the binary executable and verify

```bash
$ chmod 775 ccoctl

$ ./ccoctl help
OpenShift credentials provisioning tool
Usage:
  ccoctl [command]

Available Commands:
  aws         Manage credentials objects for AWS cloud
  azure       Manage credentials objects for Azure
  completion  Generate the autocompletion script for the specified shell
  gcp         Manage credentials objects for Google cloud
  help        Help about any command
  ibmcloud    Manage credentials objects for IBM Cloud
  nutanix     Manage credentials objects for Nutanix

Flags:
  -h, --help   help for ccoctl

Use "ccoctl [command] --help" for more information about a command.
```

### Step 2: Creating Azure resources with the ccoctl tool

See: [OpenShift Documentation - Creating Azure resources with the ccoctl tool](https://docs.redhat.com/en/documentation/openshift_container_platform/4.19/html-single/installing_on_azure/index#cco-ccoctl-creating-at-once_installing-azure-customizations)

#### 2.1 Extract Credentials Requests from OpenShift Release Image

Extract the CredentialsRequest objects from the release image:

```bash
$ oc adm release extract --cloud=azure --credentials-requests $RELEASE_IMAGE --to=credrequests
```

Output:

```bash
$ ls credrequests  
0000_26_cloud-controller-manager-operator_14_credentialsrequest-azure.yaml
0000_50_cluster-image-registry-operator_01-registry-credentials-request-azure.yaml  
0000_50_cluster-storage-operator_03_credentials_request_azure.yaml
0000_30_cluster-api_01_credentials-request.yaml
0000_50_cluster-ingress-operator_00-ingress-credentials-request.yaml                
0000_50_cluster-storage-operator_03_credentials_request_azure_file.yaml  
0000_30_machine-api-operator_00_credentials-request.yaml
0000_50_cluster-network-operator_02-cncc-credentials.yaml
```

#### 2.2 Log in to Azure CLI  

To enable the ccoctl utility to detect your Azure credentials automatically, log in to the Azure CLI:

```bash
$ az login
```

You need to prepare the Azure credentials file in advance. Below is an example of the Azure credentials file:  
```bash
$ cat <azure_credentials_file_path>/osServicePrincipal.json
{
  "subscriptionId": "REDACTED",
  "clientId": "REDACTED",
  "clientSecret": "REDACTED",
  "tenantId": "REDACTED"
}
```

Output:
Note: The output results may vary slightly depending on the Azure CLI version used here.  
```bash
$ az version
{
  "azure-cli": "2.79.0",
  "azure-cli-core": "2.79.0",
  "azure-cli-telemetry": "1.1.0",
  "extensions": {
    "ssh": "1.1.3"
  }
}

$ az login
A web browser has been opened at https://login.microsoftonline.com/organizations/oauth2/v2.0/authorize. Please continue the login in the web browser. If no web browser is available or if the web browser fails to open, use device code flow with `az login --use-device-code`.

Retrieving tenants and subscriptions for the selection...
The following tenants don't contain accessible subscriptions. Use `az login --allow-no-subscriptions` to have tenant level access.
...
[Tenant and subscription selection]

No     Subscription name    Subscription ID                       Tenant
-----  -------------------  ------------------------------------  -------------
[1] *  <Subscription Name>         <Subscription ID>  <Tenant>

The default is marked with an *; the default tenant is 'REDACTED' and subscription is 'REDACTED' (REDACTED).

Select a subscription and tenant (Type a number or Enter for no changes): 1

Tenant: <Tenant>
Subscription: <Subscription Name> (REDACTED)

[Announcements]
With the new Azure CLI login experience, you can select the subscription you want to use more easily. Learn more about it and its configuration at https://go.microsoft.com/fwlink/?linkid=2271236

If you encounter any problem, please open an issue at https://aka.ms/azclibug

[Warning] The login output has been updated. Please be aware that it no longer displays the full list of available subscriptions by default.
```

#### 2.3 Use the ccoctl tool to process all CredentialsRequest objects  
```bash
$ ccoctl azure create-all \
  --name=<azure_infra_name> \
  --installation-resource-group-name=<installation_resource_group> \
  --output-dir=<ccoctl_output_dir> \
  --region=<azure_region> \
  --subscription-id=<azure_subscription_id> \
  --credentials-requests-dir=<path_to_credentials_requests_directory> \
  --dnszone-resource-group-name=<azure_dns_zone_resource_group_name> \
  --tenant-id=<azure_tenant_id>
```

Verify the created files:

```bash
$ ls <ccoctl_output_dir>
jwks  
manifests  
openid-configuration  
serviceaccount-signer.private  
serviceaccount-signer.public  
tls

$ ls <ccoctl_output_dir>/manifests
azure-ad-pod-identity-webhook-config.yaml
cluster-authentication-02-config.yaml
openshift-cloud-controller-manager-azure-cloud-credentials-credentials.yaml
openshift-cloud-network-config-controller-cloud-credentials-credentials.yaml
openshift-cluster-csi-drivers-azure-disk-credentials-credentials.yaml
openshift-cluster-csi-drivers-azure-file-credentials-credentials.yaml
openshift-image-registry-installer-cloud-credentials-credentials.yaml
openshift-ingress-operator-cloud-credentials-credentials.yaml
openshift-machine-api-azure-cloud-credentials-credentials.yaml

$ ls <ccoctl_output_dir>/tls
bound-service-account-signing-key.key
```

### Step 3: Verify Azure Resource Groups

**Expected Resource Groups:**
- **OIDC Resource Group**: Contains Storage Account, and Managed Identities
  - Default name: `<azure_infra_name>-oidc`
  - Can be customized with `--oidc-resource-group-name`
  
- **Installation Resource Group**: Empty resource group used for cluster installation
  - Default name: `<azure_infra_name>` (if `--installation-resource-group-name` not specified, automatically generated by ccoctl)
  - Can be customized with `--installation-resource-group-name`
  - **Must remain empty** until cluster installation begins

Verify the installation resource group is still empty:  
```bash
$ az resource list --resource-group <installation_resource_group> --query "length(@)"
Expected output:  0
```  

View resources in the OIDC resource group:  
```bash
$ az resource list --resource-group <azure_infra_name>-oidc --query "[].{Name:name, Type:type}" -o table  
Expected output:
  | Name                                                                    | Type                                             |
  |-------------------------------------------------------------------------|--------------------------------------------------|
  | mihuang2911                                                             | Microsoft.Storage/storageAccounts                |
  | mihuang2911-openshift-cloud-controller-manager-azure-cloud-credentials  | Microsoft.ManagedIdentity/userAssignedIdentities |
  | mihuang2911-openshift-machine-api-azure-cloud-credentials               | Microsoft.ManagedIdentity/userAssignedIdentities |
  | mihuang2911-openshift-image-registry-installer-cloud-credentials        | Microsoft.ManagedIdentity/userAssignedIdentities |
  | mihuang2911-openshift-ingress-operator-cloud-credentials                | Microsoft.ManagedIdentity/userAssignedIdentities |
  | mihuang2911-openshift-cloud-network-config-controller-cloud-credentials | Microsoft.ManagedIdentity/userAssignedIdentities |
  | mihuang2911-openshift-cluster-csi-drivers-azure-disk-credentials        | Microsoft.ManagedIdentity/userAssignedIdentities |
  | mihuang2911-openshift-cluster-csi-drivers-azure-file-credentials        | Microsoft.ManagedIdentity/userAssignedIdentities |
```

### Step 4: Create Secret for Service Account Signing Key

Create a Secret for your private service account signing key created with `ccoctl azure create-all`:

```bash
$ oc create secret generic bound-service-account-signing-key --from-file=bound-service-account-signing-key.key=<ccoctl_output_dir>/tls/bound-service-account-signing-key.key -n <namespace>

$ oc get secret bound-service-account-signing-key -n <namespace>
NAME                                TYPE     DATA   AGE
bound-service-account-signing-key   Opaque   1      5h35m
```

### Step 5: Create Secret for Installer Manifests

Create a Secret for your installer manifests (credential role Secrets, Authentication config):

```bash
$ oc create secret generic cluster-manifests --from-file=<ccoctl_output_dir>/manifests -n <namespace>
secret/cluster-manifests created

$ oc get secret cluster-manifests -n <namespace>
NAME                TYPE     DATA   AGE
cluster-manifests   Opaque   9      5h37m
```

### Step 6: Deploy Cluster with Hive

#### 6.1 Install the Hive Operator

Refer to: [Installing Hive](install.md#installing-hive)

**Note:** When installing a cluster through ACM, the hive operator is automatically deployed once ACM is installed.

Verify Hive installation:

```bash
$ oc get pods -n hive
NAME                                READY   STATUS    RESTARTS   AGE
hive-clustersync-0                  1/1     Running   0          34s
hive-controllers-5d67988cc8-97r5p   1/1     Running   0          35s
hive-machinepool-0                  1/1     Running   0          34s
hive-operator-5c7fdd6df8-jrxvt      1/1     Running   0          3m30s
hiveadmission-5bf565bd7-nqq9h       1/1     Running   0          32s
hiveadmission-5bf565bd7-tkf4c       1/1     Running   0          32s
```

#### 6.2 Create Pull Secret

Refer to: [Using Hive - Pull Secret](using-hive.md#pull-secret)

Create the pull secret:

```bash
$ oc create secret generic cluster-pull-secret --from-file=.dockerconfigjson=/path/to/pull-secret --type=kubernetes.io/dockerconfigjson -n <namespace>
secret/cluster-pull-secret created
```

Verify the secret:

```bash
$ oc get secret cluster-pull-secret -n <namespace>
NAME                      TYPE                             DATA   AGE
cluster-pull-secret   kubernetes.io/dockerconfigjson   1      66m
```

#### 6.3 Create ClusterImageSet

Refer to: [Using Hive - OpenShift Version](using-hive.md#openshift-version)

Create a ClusterImageSet YAML file like below and apply it:

```bash
$ cat ClusterImageSets.yaml
apiVersion: hive.openshift.io/v1
kind: ClusterImageSet
metadata:
  name: <cluster-imageset>
spec:
  releaseImage: quay.io/openshift-release-dev/ocp-release:4.19.12-x86_64

$ oc apply -f ClusterImageSets.yaml
```

Verify the ClusterImageSet:
```bash
$ oc get clusterimageset
NAME                   RELEASE
cluster-imageset   quay.io/openshift-release-dev/ocp-release:4.19.12-x86_64
```

#### 6.4 Create Azure Cloud Credentials Secret

Refer to: [Using Hive - Cloud Credentials](using-hive.md#cloud-credentials)

Create the Azure credentials secret:

```bash
$ oc create secret generic azure-creds -n <namespace> --from-file=/<azure_credentials_file_path>/osServicePrincipal.json -n <namespace>
```

#### 6.5 Create InstallConfig Secret

Refer to: [Using Hive - InstallConfig](using-hive.md#installconfig)

Prepare install-config.yaml using openshift-install

```bash
Example:
$ ./openshift-install create install-config --dir <install-config-dir>
? SSH Public Key /path/to/id_rsa.pub
? Platform azure
INFO Credentials loaded from file "/<azure_credentials_file_path>/osServicePrincipal.json" 
? Region <azure_region>
? Base Domain <base_domain>
? Cluster Name <cluster_name>
? Pull Secret [? for help] ***************************
INFO Install-Config created in: <install-config-dir> 

$ ls <install-config-dir>
Install-config.yaml
```

Set credentialsMode to Manual:

```bash
$ echo "credentialsMode: Manual" >> <install-config-dir>/install-config.yaml
```

Set `platform.azure.resourceGroupName` to the installation resource group name specified with `--installation-resource-group-name` in the ccoctl command:

```bash
# Add resourceGroupName to install-config.yaml
sed -i "/^  azure:/a\    resourceGroupName: ${RG_NAME}" <install-config-dir>/install-config.yaml
```

Verify the install-config.yaml contains the correct resource group:

```bash
$ cat <install-config-dir>/install-config.yaml | grep -A 5 "azure:"
  azure:
    resourceGroupName: <installation_resource_group>
    baseDomainResourceGroupName: <dns_zone_resource_group>
    region: <azure_region>
    cloudName: AzurePublicCloud
```

Create install-config secret:

```bash
$ oc create secret generic install-config-secret --from-file=install-config.yaml=<install-config-dir>/install-config.yaml -n <namespace>
```

#### 6.6 Deploy ClusterDeployment

Refer to: [Using Hive - ClusterDeployment](using-hive.md#clusterdeployment)

**Important configurations:**
- Set `spec.boundServiceAccountSigningKeySecretRef.name` to point to the Secret created in Step 4 (`bound-service-account-signing-key`)
- Set `spec.provisioning.manifestsSecretRef` to point to the Secret created in Step 5 (`cluster-manifests`)
- Set `spec.pullSecretRef` to point to the Secret created in Step 6.2 (`cluster-pull-secret`)
- Set `spec.provisioning.imageSetRef` to point to the ClusterImageSet created in Step 6.3 (`cluster-imageset`)
- Set `platform.azure.credentialsSecretRef.name` to point to the Secret created in Step 6.4 (`azure-creds`)
- Set `spec.provisioning.installConfigSecretRef` to point to the Secret created in Step 6.5 (`install-config-secret`)

**Example ClusterDeployment YAML:**

```yaml
apiVersion: hive.openshift.io/v1
kind: ClusterDeployment
metadata:
  creationTimestamp: null
  name: cluster-name
  namespace: namespace
spec:
  baseDomain: base_domain
  boundServiceAccountSigningKeySecretRef:
    name: bound-service-account-signing-key
  clusterName: cluster-name
  controlPlaneConfig:
    servingCertificates: {}
  installed: false
  platform:
    azure:
      baseDomainResourceGroupName: xxx
      cloudName: AzurePublicCloud
      credentialsSecretRef:
        name: azure-creds
      region: azure_region
  provisioning:
    imageSetRef:
      name:  cluster-imageset
    installConfigSecretRef:
      name: install-config-secret
    manifestsSecretRef:
      name: cluster-manifests
  pullSecretRef:
    name: cluster-pull-secret
  installAttemptsLimit: 1
status: {}
```

**Deploy the ClusterDeployment:**

```bash
$ oc apply -f ClusterDeployment.yaml
```

**Note:** Setting `installAttemptsLimit: 1` prevents automatic retries when cluster installation fails. Without this setting, automatic retries will first clean up resources from the initial installation attempt, which deletes the installation resource group. The retry will then fail because the resource group no longer exists and Role Assignments are not restored automatically.

**Recovery Steps After Failed Provision:**  
If cluster provision fails:
1. Delete the failed ClusterDeployment: `oc delete clusterdeployment <cluster-name> -n <namespace>`, this will delete the installation resource group.
2. Re-run `ccoctl azure create-all` with the same parameters used in Step 2.3. This will recreate the deleted installation resource group and restore Role Assignments, granting Managed Identities permissions at the resource group scope.
3. Redeploy the ClusterDeployment after fixing the issue that caused the failure


### Step 7: Verify Cluster Installation

Check the ClusterDeployment status:

```bash
$ oc get cd -n <namespace>
NAME          INFRAID             PLATFORM   REGION      VERSION   CLUSTERTYPE   PROVISIONSTATUS   POWERSTATE   AGE
mihuang2911   mihuang2911-rtn8f   azure      centralus   4.19.12                 Provisioned       Running      155m
```

#### Login to Spoke Cluster and Verify Configuration

1. Verify Authentication configuration:

```bash
$ oc get authentication cluster -o yaml
apiVersion: config.openshift.io/v1
kind: Authentication
metadata:
  creationTimestamp: "2025-09-17T01:47:18Z"
  generation: 2
  name: cluster
  resourceVersion: "26374"
  uid: 615bb8bf-bfae-43f8-8568-128c908419b4
spec:
  oauthMetadata:
    name: ""
  serviceAccountIssuer: https://mihuang2911.blob.core.windows.net/mihuang2911
  type: ""
  webhookTokenAuthenticator:
    kubeConfig:
      name: webhook-authentication-integrated-oauth
status:
  integratedOAuthMetadata:
    name: oauth-openshift
```

2. Check that the cluster's cloud credentials are configured in manual mode:

```bash
$ oc get cloudcredential cluster -o yaml
apiVersion: operator.openshift.io/v1
kind: CloudCredential
metadata:
  creationTimestamp: "2025-09-17T01:46:48Z"
  generation: 1
  name: cluster
  resourceVersion: "511"
  uid: ac18b72a-0aa8-4261-b0ec-39293e824b86
spec:
  credentialsMode: Manual
  logLevel: Normal
  operatorLogLevel: Normal
```

3. Ensure the cluster does not have root credentials:

```bash
$ oc get secrets -n kube-system azure-credentials
Error from server (NotFound): secrets "azure-credentials" not found
```

4. Check that the secret contains the azure_federated_token_file key:

```bash
$ oc get secrets -n openshift-image-registry installer-cloud-credentials -o jsonpath='{.data}' | jq
{
  "azure_client_id": "REDACTED",
  "azure_federated_token_file": "REDACTED",
  "azure_region": "REDACTED",
  "azure_subscription_id": "REDACTED",
  "azure_tenant_id": "REDACTED"
}
```

5. Ensure that the Pod Identity Webhook and Cloud Credential Operator pods are running:

```bash
$ oc get po -n openshift-cloud-credential-operator
NAME                                         READY   STATUS    RESTARTS   AGE
cloud-credential-operator-5c7bc7b9bf-q96n8   2/2     Running   0          23m
pod-identity-webhook-55b5df8446-6tg2q        1/1     Running   0          19m
pod-identity-webhook-55b5df8446-kwqlj        1/1     Running   0          19m
```

### Step 8: Cleanup Azure resources after uninstalling the cluster
Make sure you clean up the following resources after you uninstall your cluster. You can use the <azure_infra_name> used in installation step 2 to identify these resources.
```bash
./ccoctl azure delete --name <azure_infra_name> --region <azure_region> --subscription-id <azure_subscription_id> --delete-oidc-resource-group
```

## Procedure 2: Microsoft Entra Workload ID Cluster Installation using hiveutil

**Note:** `hiveutil` is not customer supported.   
For detailed information about `hiveutil` usage and available options, see [hiveutil documentation](./hiveutil.md).

### Steps 1-5: Same as [Procedure 1: Microsoft Entra Workload ID Cluster Installation](#procedure-1-microsoft-entra-workload-id-cluster-installation)

Follow Steps 1-5 from Procedure 1 to extract ccoctl binary, create Azure resources, and prepare the required secrets.

### Step 6: Deploy Cluster using hiveutil

#### 6.1 Extract hiveutil binary

**Note:** To find the latest Hive operator image, visit the [Hive operator image repository on Quay.io](https://quay.io/repository/redhat-user-workloads/crt-redhat-acm-tenant/hive-operator/hive?tab=tags&tag=latest). You need to select an image with the `hive-on-push` tag (e.g., `hive-on-push-tlzm5-build-image-index`) to use in the command below.

Extract the hiveutil binary from the Hive operator image:

```bash
$ oc image extract quay.io/redhat-user-workloads/crt-redhat-acm-tenant/hive-operator/hive:hive-on-push-dlrb8-build-image-index --path /usr/bin/hiveutil:./ --confirm
$ chmod +x hiveutil
$ ./hiveutil -h
```

**Note:** The `chmod +x hiveutil` command is required because the extracted binary does not have execute permissions by default.

#### 6.2 Generate manifests using hiveutil

Generate the cluster manifests using hiveutil:

```bash
$ ./hiveutil create-cluster cluster-name \
  --cloud=azure \
  --region=<azure_region> \
  --base-domain=<base_domain> \
  --install-once \
  --release-image=$RELEASE_IMAGE \
  --creds-file=/path/to/credentials-file \
  --pull-secret-file=/path/to/pull-secret -o yaml > clustermanifests.yaml
```

**Note:**   
If you don't specify the `--pull-secret-file` parameter, `hiveutil` will automatically look for the pull secret at the default location `~/.pull-secret`. For more details about other pull secret configuration options (environment variable, command line parameter, etc.), refer to the [Pull Secret section in hiveutil documentation](./hiveutil.md#pull-secret).  
For the `--creds-file` parameter, if you don't specify it, `hiveutil` will automatically look for the credentials at the default location `~/.azure/osServicePrincipal.json`. 

#### 6.3 Update ClusterDeployment and InstallConfig

Update the generated `clustermanifests.yaml` file with the following changes:

- In your InstallConfig set `credentialsMode: Manual`
- In your InstallConfig, set `platform.azure.resourceGroupName` to the installation resource group created by `ccoctl azure create-all` (Step 3)
- In your ClusterDeployment set `spec.boundServiceAccountSigningKeySecretRef.name` to point to the Secret created above (`bound-service-account-signing-key`)
- In your ClusterDeployment set `spec.provisioning.manifestsSecretRef` to point to the Secret created above (`cluster-manifests`)

#### 6.4 Deploy Cluster

Deploy the cluster:

```bash
$ oc apply -f clustermanifests.yaml
```

### Step 7-8: Same as [Procedure 1: Microsoft Entra Workload ID Cluster Installation](#procedure-1-microsoft-entra-workload-id-cluster-installation)
Follow Steps 7-8 from Procedure 1 to check the installed cluster and Azure resources cleanup.