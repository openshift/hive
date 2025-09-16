# Test Case: Ability to Override httpCustomErrorCodePages in ClusterDeployment

## Test Information
- **Test ID**: OCP-83946
- **Title**: Ability to Override httpCustomErrorCodePages in ClusterDeployment
- **User Story**: [HIVE-2040](https://issues.redhat.com/browse/HIVE-2040)
- **Priority**: Medium
- **Status**: Draft
- **Type**: Test Case

## Test Metadata
- **Component**: Hive
- **Importance**: Critical
- **Level**: Component
- **Automation**: Not Automated
- **Type**: Positive
- **Test Type**: Functional
- **Subteam**: Cluster Operator
- **Products**: OCP
- **Version**: 4.20
- **Trello**: HIVE-2040

## Test Steps

### Step A: Create cluster with empty httpErrorCodePages

**Action:**
Create a cluster with httpErrorCodePages.name set as empty.

```yaml
ingress:
- domain: apps.{CLUSTER_NAME}.qe.devcluster.openshift.com
  httpErrorCodePages:
    name: ""
    name: default
```

**Expected Result:**
Cluster installation completes successfully and reaches "Provisioned" status with "Running" power state.

### Step 1: Verify empty httpErrorCodePages in SyncSet

**Action:**
Verify that the httpErrorCodePages.name field in the SyncSet is an empty string "".

**Expected Result:**
SyncSet is created with httpErrorCodePages.name field set to empty string "".

### Step 2: Check IngressController in spoke cluster

**Action:**
Log in to the spoke cluster and check the IngressController configuration.

**Expected Result:**
IngressController in spoke cluster has httpErrorCodePages.name field set to empty string "".

### Step B: Create cluster with custom httpErrorCodePages

**Action:**
Create a ClusterDeployment with httpErrorCodePages.name set to a value, for example "custom-error-pages".

```yaml
CD.spec.ingress:
ingress:
- domain: apps.{CLUSTER_NAME_2}.qe.devcluster.openshift.com
  httpErrorCodePages:
    name: custom-error-pages-test
    name: default
```

**Expected Result:**
Cluster provision completes and reaches "Provisioned" status, but may be in "WaitingForClusterOperators" state initially.

### Step 3: Verify custom httpErrorCodePages in SyncSet

**Action:**
Verify that the httpErrorCodePages.name field in the SyncSet is custom-error-pages-test.

**Expected Result:**
SyncSet is created with httpErrorCodePages.name field set to "custom-error-pages-test".

### Step 4: Check IngressController with custom value

**Action:**
Log in to the spoke cluster and check the IngressController configuration.

**Expected Result:**
IngressController in spoke cluster has httpErrorCodePages.name field set to "custom-error-pages-test".

### Step 5: Create and apply SelectorSyncSet

**Action:**
Create and apply a SelectorSyncSet configuration file.

```yaml
# cat SelectorSyncSet-2040.yaml
apiVersion: hive.openshift.io/v1
kind: SelectorSyncSet
metadata:
  name: httperror-configmaps
spec:
  clusterDeploymentSelector:
    matchLabels:
      httperror: enabled
  resourceApplyMode: Sync
  resources:
  - apiVersion: v1
    kind: ConfigMap
    metadata:
      name: custom-error-pages-test
      namespace: openshift-config
    data:
      404.html: |
        <html><body><h1>My Custom 404 Page</h1></body></html>
      503.html: |
        <html><body><h1>My Custom 503 Page</h1></body></html>

# oc apply -f SelectorSyncSet-2040.yaml
```

**Expected Result:**
SelectorSyncSet is successfully applied and ConfigMap "custom-error-pages-test" is created in the openshift-config namespace with 2 data entries.

### Step 6: Wait for cluster to reach Running state

**Action:**
Wait for the ConfigMap to take effect and for the cluster to reach the Running state.

**Expected Result:**
Cluster reaches "Running" power state indicating all components are operational.

### Step 7: Check Cluster Operators are Ready

**Action:**
Log in to the spoke cluster and check that all Cluster Operators are Ready.

**Expected Result:**
All Cluster Operators are in "Available=True" state with "Progressing=False" and "Degraded=False", indicating the cluster is fully operational.