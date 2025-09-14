# Test Case: HIVE-2883
**Component:** hive
**Summary:** [FeatureGate] Hive-Support multiple NICs in Nutanix

## Test Overview
- **Total Test Cases:** 3
- **Test Types:** End-to-End, Integration, Error Handling
- **Estimated Time:** 90 minutes

## Test Cases

### Test Case HIVE-2883_001
**Name:** Verify Hive supports multi-NIC Nutanix cluster provisioning with multiple subnets
**Description:** Validate that Hive can successfully provision a Nutanix cluster with multiple network interfaces using multiple subnetUUIDs in ClusterDeployment configuration
**Type:** End-to-End
**Priority:** High

#### Prerequisites
- Nutanix cluster with Prism Central accessible
- Multiple subnets configured in Nutanix environment (minimum 2 subnets)
- Valid Nutanix credentials stored in hive-nutanix-creds secret
- MultiSubnet FeatureGate enabled in OpenShift installer
- OpenShift 4.18+ with OCPSTRAT-1532 support
- Hive operator deployed and functional

#### Test Steps
1. **Action:** Create Nutanix credentials secret for Hive
   ```bash
   oc create secret generic hive-nutanix-creds \
     --from-literal=username="prism-central-user" \
     --from-literal=password="prism-central-password" \
     -n hive
   ```
   **Expected:** Secret created successfully with exit code 0

2. **Action:** Create ClusterDeployment with multiple Nutanix subnets configuration
   ```bash
   cat << EOF | oc apply -f -
   apiVersion: hive.openshift.io/v1
   kind: ClusterDeployment
   metadata:
     name: hive2883-multi-nic-test
     namespace: hive
   spec:
     clusterName: hive2883-multi-nic-test
     baseDomain: nutanix.example.com
     platform:
       nutanix:
         apiVIPs:
         - "10.0.1.100"
         ingressVIPs:
         - "10.0.1.101"
         prismCentral:
           endpoint:
             address: "prism-central.nutanix.com"
             port: 9440
           username: "prism-central-user"
           password:
             name: hive-nutanix-creds
             key: password
         prismElements:
         - endpoint:
             address: "prism-element.nutanix.com"
             port: 9440
           uuid: "12345678-1234-1234-1234-123456789012"
         subnetUUIDs:
         - "subnet-uuid-1234-5678-90ab"
         - "subnet-uuid-cdef-1234-5678"
         failureDomains:
         - name: "nutanix-fd-1"
           prismElement:
             uuid: "12345678-1234-1234-1234-123456789012"
           subnetUUIDs:
           - "subnet-uuid-1234-5678-90ab"
         - name: "nutanix-fd-2"
           prismElement:
             uuid: "12345678-1234-1234-1234-123456789012"
           subnetUUIDs:
           - "subnet-uuid-cdef-1234-5678"
     provisioning:
       installConfigSecretRef:
         name: hive2883-install-config
       imageSetRef:
         name: openshift-v4.18.0
   EOF
   ```
   **Expected:** ClusterDeployment created with status.conditions[type=ClusterDeploymentInitialized].status=True

3. **Action:** Verify ClusterDeployment accepts multi-subnet configuration
   ```bash
   # Count configured subnets in ClusterDeployment
   SUBNET_COUNT=$(oc get clusterdeployment hive2883-multi-nic-test -n hive -o jsonpath='{.spec.platform.nutanix.subnetUUIDs[*]}' | wc -w)
   echo "Configured subnets: $SUBNET_COUNT"

   # Verify failure domains configuration
   FD_COUNT=$(oc get clusterdeployment hive2883-multi-nic-test -n hive -o jsonpath='{.spec.platform.nutanix.failureDomains[*].name}' | wc -w)
   echo "Configured failure domains: $FD_COUNT"
   ```
   **Expected:** SUBNET_COUNT ≥ 2 and FD_COUNT ≥ 2, confirming multi-subnet configuration accepted

4. **Action:** Monitor cluster installation progress and validate provisioning status
   ```bash
   # Wait for installation to start (timeout: 10 minutes)
   timeout 600 bash -c 'until oc get clusterdeployment hive2883-multi-nic-test -n hive -o jsonpath="{.status.conditions[?(@.type==\"ProvisionStarted\")].status}" | grep -q "True"; do sleep 30; done'

   # Check installation progress
   oc get clusterdeployment hive2883-multi-nic-test -n hive -o jsonpath='{.status.conditions[?(@.type=="Provisioned")].status}'
   ```
   **Expected:** ProvisionStarted condition becomes True within 10 minutes, indicating successful start of multi-NIC cluster provisioning

5. **Action:** Validate cluster nodes have multiple network interfaces after successful installation
   ```bash
   # Wait for cluster to be provisioned (timeout: 60 minutes)
   timeout 3600 bash -c 'until oc get clusterdeployment hive2883-multi-nic-test -n hive -o jsonpath="{.status.conditions[?(@.type==\"Provisioned\")].status}" | grep -q "True"; do sleep 60; done'

   # Extract cluster admin kubeconfig
   oc extract secret/$(oc get clusterdeployment hive2883-multi-nic-test -n hive -o jsonpath='{.spec.clusterMetadata.adminKubeconfigSecretRef.name}') -n hive --to=/tmp --keys=kubeconfig

   # Verify nodes have multiple network interfaces
   KUBECONFIG=/tmp/kubeconfig oc get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")]}' | jq length

   # Check node network configuration
   KUBECONFIG=/tmp/kubeconfig oc get nodes -o jsonpath='{.items[*].metadata.annotations.machine\.openshift\.io/machine}' | head -1 | xargs -I {} oc get machine {} -o jsonpath='{.spec.providerSpec.value.subnets[*].uuid}' | wc -w
   ```
   **Expected:** Each node shows ≥ 2 network interfaces configured, confirming successful multi-NIC deployment

### Test Case HIVE-2883_002
**Name:** Verify FeatureGate dependency for multi-NIC Nutanix support
**Description:** Validate that multi-NIC functionality requires proper FeatureGate configuration and behaves correctly when disabled
**Type:** Integration
**Priority:** Medium

#### Prerequisites
- Nutanix test environment with multiple subnets available
- Ability to control MultiSubnet FeatureGate configuration
- Hive operator with configurable FeatureGate settings
- Test cluster deployment configuration ready

#### Test Steps
1. **Action:** Verify current FeatureGate configuration for MultiSubnet
   ```bash
   # Check if MultiSubnet feature is enabled in cluster
   oc get featuregate cluster -o jsonpath='{.spec.featureSet}'

   # Look for MultiSubnet in enabled features
   oc get featuregate cluster -o jsonpath='{.status.conditions[?(@.type=="MultiSubnet")].status}' 2>/dev/null || echo "MultiSubnet FeatureGate not found"
   ```
   **Expected:** MultiSubnet FeatureGate status is clearly identifiable as enabled or disabled

2. **Action:** Attempt to create ClusterDeployment with multi-subnet configuration when FeatureGate is properly enabled
   ```bash
   cat << EOF | oc apply -f -
   apiVersion: hive.openshift.io/v1
   kind: ClusterDeployment
   metadata:
     name: hive2883-featuregate-test
     namespace: hive
   spec:
     clusterName: hive2883-featuregate-test
     baseDomain: nutanix.example.com
     platform:
       nutanix:
         subnetUUIDs:
         - "subnet-uuid-1234-5678-90ab"
         - "subnet-uuid-cdef-1234-5678"
         failureDomains:
         - name: "fd-1"
           subnetUUIDs: ["subnet-uuid-1234-5678-90ab"]
         - name: "fd-2"
           subnetUUIDs: ["subnet-uuid-cdef-1234-5678"]
   EOF
   ```
   **Expected:** ClusterDeployment creation succeeds with multi-subnet configuration when FeatureGate is enabled

3. **Action:** Monitor installation behavior with FeatureGate enabled
   ```bash
   # Check for any FeatureGate-related error messages in ClusterDeployment status
   oc get clusterdeployment hive2883-featuregate-test -n hive -o jsonpath='{.status.conditions[?(@.type=="InstallLaunchError")].message}' 2>/dev/null || echo "No install launch errors"

   # Verify installer can process multi-subnet configuration
   oc logs job/$(oc get job -n hive -l hive.openshift.io/cluster-deployment-name=hive2883-featuregate-test -o name | head -1) -n hive | grep -i "subnet\|featuregate\|multinic" | head -10
   ```
   **Expected:** No FeatureGate-related errors in ClusterDeployment status; installer logs show successful processing of multi-subnet configuration

### Test Case HIVE-2883_003
**Name:** Verify error handling for invalid multi-NIC Nutanix configurations
**Description:** Validate proper error reporting and recovery when invalid subnet configurations are provided in multi-NIC setup
**Type:** Error Handling
**Priority:** Medium

#### Prerequisites
- Nutanix test environment configured
- Hive operator deployed and functional
- Knowledge of invalid/non-existent subnet UUIDs for testing
- Valid credentials for Nutanix cluster

#### Test Steps
1. **Action:** Create ClusterDeployment with invalid subnet UUIDs
   ```bash
   cat << EOF | oc apply -f -
   apiVersion: hive.openshift.io/v1
   kind: ClusterDeployment
   metadata:
     name: hive2883-invalid-subnet-test
     namespace: hive
   spec:
     clusterName: hive2883-invalid-subnet-test
     baseDomain: nutanix.example.com
     platform:
       nutanix:
         subnetUUIDs:
         - "invalid-subnet-uuid-1111"
         - "invalid-subnet-uuid-2222"
         failureDomains:
         - name: "fd-invalid-1"
           subnetUUIDs: ["invalid-subnet-uuid-1111"]
         - name: "fd-invalid-2"
           subnetUUIDs: ["invalid-subnet-uuid-2222"]
   EOF
   ```
   **Expected:** ClusterDeployment creation succeeds but shows validation errors in status

2. **Action:** Monitor error detection and reporting for invalid configuration
   ```bash
   # Wait for error conditions to appear (timeout: 15 minutes)
   timeout 900 bash -c 'until oc get clusterdeployment hive2883-invalid-subnet-test -n hive -o jsonpath="{.status.conditions[?(@.type==\"InstallLaunchError\")].status}" | grep -q "True"; do sleep 30; done'

   # Extract error message for validation
   ERROR_MSG=$(oc get clusterdeployment hive2883-invalid-subnet-test -n hive -o jsonpath='{.status.conditions[?(@.type=="InstallLaunchError")].message}')
   echo "Error message: $ERROR_MSG"

   # Verify error message contains subnet-related information
   echo "$ERROR_MSG" | grep -i "subnet" && echo "Subnet error detected" || echo "No subnet error found"
   ```
   **Expected:** InstallLaunchError condition appears within 15 minutes with clear error message containing "subnet" reference, indicating proper validation of invalid subnet UUIDs

3. **Action:** Verify cleanup and recovery by correcting configuration
   ```bash
   # Delete the invalid ClusterDeployment
   oc delete clusterdeployment hive2883-invalid-subnet-test -n hive

   # Verify clean deletion
   oc get clusterdeployment hive2883-invalid-subnet-test -n hive 2>/dev/null && echo "Deletion failed" || echo "Deletion successful"

   # Create corrected ClusterDeployment with valid subnets
   cat << EOF | oc apply -f -
   apiVersion: hive.openshift.io/v1
   kind: ClusterDeployment
   metadata:
     name: hive2883-corrected-test
     namespace: hive
   spec:
     clusterName: hive2883-corrected-test
     baseDomain: nutanix.example.com
     platform:
       nutanix:
         subnetUUIDs:
         - "valid-subnet-uuid-1234"
         - "valid-subnet-uuid-5678"
   EOF
   ```
   **Expected:** Invalid ClusterDeployment deletes cleanly, and corrected ClusterDeployment creates successfully without InstallLaunchError condition

4. **Action:** Validate recovery and normal operation with corrected configuration
   ```bash
   # Monitor corrected deployment for successful initialization
   timeout 300 bash -c 'until oc get clusterdeployment hive2883-corrected-test -n hive -o jsonpath="{.status.conditions[?(@.type==\"ClusterDeploymentInitialized\")].status}" | grep -q "True"; do sleep 15; done'

   # Verify no error conditions present
   ERROR_CONDITIONS=$(oc get clusterdeployment hive2883-corrected-test -n hive -o jsonpath='{.status.conditions[?(@.status=="True" && @.type=~".*Error.*")].type}')
   echo "Error conditions: $ERROR_CONDITIONS"

   # Confirm configuration acceptance
   ACCEPTED_SUBNETS=$(oc get clusterdeployment hive2883-corrected-test -n hive -o jsonpath='{.spec.platform.nutanix.subnetUUIDs[*]}' | wc -w)
   echo "Accepted subnets count: $ACCEPTED_SUBNETS"
   ```
   **Expected:** ClusterDeploymentInitialized condition becomes True within 5 minutes, no error conditions present, and ACCEPTED_SUBNETS = 2, confirming successful recovery and acceptance of valid multi-subnet configuration

---
*Generated from Markdown template*