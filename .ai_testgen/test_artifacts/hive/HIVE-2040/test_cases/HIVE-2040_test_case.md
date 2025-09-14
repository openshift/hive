# Test Case: HIVE-2040
**Component:** hive
**Summary:** Add HttpErrorCodePages to CD.Spec.Ingress[]

## Test Overview
- **Total Test Cases:** 4
- **Test Types:** Positive, Negative, Multi-Configuration, Update Scenario
- **Estimated Time:** 45 minutes

## Test Cases

### Test Case HIVE-2040_001
**Name:** Verify HttpErrorCodePages field propagates to IngressController when configured
**Description:** Validate that HttpErrorCodePages configuration in ClusterDeployment.Spec.Ingress[] correctly propagates to the IngressController in the spoke cluster
**Type:** Positive
**Priority:** High

#### Prerequisites
- Hive management cluster with RemoteClusterIngress controller running
- Target spoke cluster accessible and healthy
- ConfigMap with custom error page content exists in openshift-config namespace:
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: custom-error-pages-hive2040
  namespace: openshift-config
data:
  error-page-503.http: |
    HTTP/1.0 503 Service Unavailable
    Content-Type: text/html

    <html><body><h1>Custom Service Unavailable</h1></body></html>
```
- Cluster admin access to both management and spoke clusters

#### Test Steps
1. **Action:** Create ClusterDeployment with HttpErrorCodePages configuration in Spec.Ingress[]
   ```bash
   cat <<EOF | oc apply -f -
   apiVersion: hive.openshift.io/v1
   kind: ClusterDeployment
   metadata:
     name: test-cluster-hive2040
     namespace: hive-test
   spec:
     clusterName: test-cluster-hive2040
     platform:
       aws:
         region: us-east-1
     ingress:
     - name: default
       domain: "*.apps.test-hive2040.example.com"
       httpErrorCodePages:
         name: custom-error-pages-hive2040
     installed: true
   EOF
   ```
   **Expected:** ClusterDeployment created successfully, HttpErrorCodePages field accepted without validation errors

2. **Action:** Verify SyncSet generation contains IngressController with HttpErrorCodePages
   ```bash
   # Wait for SyncSet creation (up to 60 seconds)
   timeout 60 bash -c 'until oc get syncset test-cluster-hive2040-clusteringress -n hive-test; do sleep 5; done'

   # Extract and verify IngressController configuration
   SYNCSET_CONTENT=$(oc get syncset test-cluster-hive2040-clusteringress -n hive-test -o jsonpath='{.spec.resources[0].object}')
   echo "$SYNCSET_CONTENT" | jq -r '.spec.httpErrorCodePages.name'
   ```
   **Expected:** SyncSet created within 60 seconds, IngressController object contains httpErrorCodePages.name="custom-error-pages-hive2040"

3. **Action:** Validate IngressController creation in spoke cluster with correct HttpErrorCodePages
   ```bash
   # Query spoke cluster IngressController
   oc --kubeconfig=${SPOKE_KUBECONFIG} get ingresscontroller default -n openshift-ingress-operator -o jsonpath='{.spec.httpErrorCodePages.name}'

   # Verify IngressController has correct configuration
   ACTUAL_CONFIG=$(oc --kubeconfig=${SPOKE_KUBECONFIG} get ingresscontroller default -n openshift-ingress-operator -o jsonpath='{.spec.httpErrorCodePages.name}')
   [[ "$ACTUAL_CONFIG" == "custom-error-pages-hive2040" ]] && echo "PASS: HttpErrorCodePages propagated correctly" || echo "FAIL: Expected custom-error-pages-hive2040, got $ACTUAL_CONFIG"
   ```
   **Expected:** IngressController in spoke cluster has httpErrorCodePages.name="custom-error-pages-hive2040", validation passes with "PASS" message

4. **Action:** Verify configuration persistence through reconciliation cycles
   ```bash
   # Trigger reconciliation by adding annotation
   oc annotate clusterdeployment test-cluster-hive2040 -n hive-test test.reconcile.timestamp="$(date +%s)"

   # Wait and verify configuration persists (check 3 times over 90 seconds)
   for i in {1..3}; do
     sleep 30
     CURRENT_CONFIG=$(oc --kubeconfig=${SPOKE_KUBECONFIG} get ingresscontroller default -n openshift-ingress-operator -o jsonpath='{.spec.httpErrorCodePages.name}')
     echo "Check $i: HttpErrorCodePages = $CURRENT_CONFIG"
     [[ "$CURRENT_CONFIG" == "custom-error-pages-hive2040" ]] || exit 1
   done
   echo "PASS: Configuration persisted through reconciliation cycles"
   ```
   **Expected:** Configuration remains "custom-error-pages-hive2040" across all 3 checks, displays "PASS: Configuration persisted through reconciliation cycles"

### Test Case HIVE-2040_002
**Name:** Verify IngressController has empty HttpErrorCodePages when not configured
**Description:** Validate default behavior when HttpErrorCodePages is not specified in ClusterDeployment.Spec.Ingress[]
**Type:** Negative
**Priority:** Medium

#### Prerequisites
- Hive management cluster with RemoteClusterIngress controller running
- Target spoke cluster accessible and healthy
- No existing ClusterDeployment with same name
- Cluster admin access to both management and spoke clusters

#### Test Steps
1. **Action:** Create ClusterDeployment without HttpErrorCodePages configuration
   ```bash
   cat <<EOF | oc apply -f -
   apiVersion: hive.openshift.io/v1
   kind: ClusterDeployment
   metadata:
     name: test-cluster-hive2040-default
     namespace: hive-test
   spec:
     clusterName: test-cluster-hive2040-default
     platform:
       aws:
         region: us-east-1
     ingress:
     - name: default
       domain: "*.apps.test-default.example.com"
     installed: true
   EOF
   ```
   **Expected:** ClusterDeployment created successfully without HttpErrorCodePages field

2. **Action:** Verify SyncSet generation contains IngressController without HttpErrorCodePages
   ```bash
   # Wait for SyncSet creation
   timeout 60 bash -c 'until oc get syncset test-cluster-hive2040-default-clusteringress -n hive-test; do sleep 5; done'

   # Check that HttpErrorCodePages field is absent
   SYNCSET_CONTENT=$(oc get syncset test-cluster-hive2040-default-clusteringress -n hive-test -o json)
   HAS_ERROR_PAGES=$(echo "$SYNCSET_CONTENT" | jq -r '.spec.resources[0].object.spec.httpErrorCodePages // "null"')
   [[ "$HAS_ERROR_PAGES" == "null" ]] && echo "PASS: HttpErrorCodePages field absent as expected" || echo "FAIL: HttpErrorCodePages field present: $HAS_ERROR_PAGES"
   ```
   **Expected:** SyncSet created, IngressController object does not contain httpErrorCodePages field, validation shows "PASS: HttpErrorCodePages field absent as expected"

3. **Action:** Validate spoke cluster IngressController has empty/missing HttpErrorCodePages
   ```bash
   # Verify IngressController lacks HttpErrorCodePages configuration
   SPOKE_ERROR_PAGES=$(oc --kubeconfig=${SPOKE_KUBECONFIG} get ingresscontroller default -n openshift-ingress-operator -o jsonpath='{.spec.httpErrorCodePages.name}' 2>/dev/null || echo "")
   [[ -z "$SPOKE_ERROR_PAGES" ]] && echo "PASS: IngressController has no HttpErrorCodePages as expected" || echo "FAIL: Unexpected HttpErrorCodePages: $SPOKE_ERROR_PAGES"
   ```
   **Expected:** IngressController in spoke cluster has no httpErrorCodePages field, validation shows "PASS: IngressController has no HttpErrorCodePages as expected"

### Test Case HIVE-2040_003
**Name:** Verify multiple ingress controllers with different HttpErrorCodePages configurations
**Description:** Validate that multiple ClusterIngress entries can have different HttpErrorCodePages configurations that propagate correctly to respective IngressController resources
**Type:** Multi-Configuration
**Priority:** High

#### Prerequisites
- Hive management cluster with RemoteClusterIngress controller running
- Target spoke cluster accessible and healthy
- Two ConfigMaps with different error page content exist:
```yaml
# ConfigMap 1
apiVersion: v1
kind: ConfigMap
metadata:
  name: default-error-pages-hive2040
  namespace: openshift-config
data:
  error-page-503.http: |
    <html><body><h1>Default Service Unavailable</h1></body></html>
---
# ConfigMap 2
apiVersion: v1
kind: ConfigMap
metadata:
  name: custom-error-pages-hive2040
  namespace: openshift-config
data:
  error-page-503.http: |
    <html><body><h1>Custom Service Unavailable</h1></body></html>
```
- Cluster admin access to both management and spoke clusters

#### Test Steps
1. **Action:** Create ClusterDeployment with multiple ingress configurations having different HttpErrorCodePages
   ```bash
   cat <<EOF | oc apply -f -
   apiVersion: hive.openshift.io/v1
   kind: ClusterDeployment
   metadata:
     name: test-cluster-hive2040-multi
     namespace: hive-test
   spec:
     clusterName: test-cluster-hive2040-multi
     platform:
       aws:
         region: us-east-1
     ingress:
     - name: default
       domain: "*.apps.test-multi.example.com"
       httpErrorCodePages:
         name: default-error-pages-hive2040
     - name: custom
       domain: "*.custom.test-multi.example.com"
       httpErrorCodePages:
         name: custom-error-pages-hive2040
     installed: true
   EOF
   ```
   **Expected:** ClusterDeployment created successfully with multiple ingress configurations

2. **Action:** Verify SyncSet contains multiple IngressController resources with correct HttpErrorCodePages
   ```bash
   # Wait for SyncSet creation
   timeout 60 bash -c 'until oc get syncset test-cluster-hive2040-multi-clusteringress -n hive-test; do sleep 5; done'

   # Extract both IngressController configurations
   SYNCSET_JSON=$(oc get syncset test-cluster-hive2040-multi-clusteringress -n hive-test -o json)

   # Count IngressController resources
   RESOURCE_COUNT=$(echo "$SYNCSET_JSON" | jq '.spec.resources | length')
   [[ "$RESOURCE_COUNT" == "2" ]] && echo "PASS: Found 2 IngressController resources" || echo "FAIL: Expected 2 resources, found $RESOURCE_COUNT"

   # Verify each IngressController has correct HttpErrorCodePages
   DEFAULT_ERROR_PAGES=$(echo "$SYNCSET_JSON" | jq -r '.spec.resources[] | select(.object.metadata.name=="default") | .object.spec.httpErrorCodePages.name')
   CUSTOM_ERROR_PAGES=$(echo "$SYNCSET_JSON" | jq -r '.spec.resources[] | select(.object.metadata.name=="custom") | .object.spec.httpErrorCodePages.name')

   [[ "$DEFAULT_ERROR_PAGES" == "default-error-pages-hive2040" ]] && echo "PASS: Default IngressController has correct HttpErrorCodePages" || echo "FAIL: Default expected default-error-pages-hive2040, got $DEFAULT_ERROR_PAGES"
   [[ "$CUSTOM_ERROR_PAGES" == "custom-error-pages-hive2040" ]] && echo "PASS: Custom IngressController has correct HttpErrorCodePages" || echo "FAIL: Custom expected custom-error-pages-hive2040, got $CUSTOM_ERROR_PAGES"
   ```
   **Expected:** SyncSet contains 2 IngressController resources, each with correct respective HttpErrorCodePages configuration, all validations show "PASS"

3. **Action:** Validate both IngressController resources exist in spoke cluster with correct configurations
   ```bash
   # Verify default IngressController
   DEFAULT_SPOKE_CONFIG=$(oc --kubeconfig=${SPOKE_KUBECONFIG} get ingresscontroller default -n openshift-ingress-operator -o jsonpath='{.spec.httpErrorCodePages.name}')
   [[ "$DEFAULT_SPOKE_CONFIG" == "default-error-pages-hive2040" ]] && echo "PASS: Default IngressController correctly configured in spoke cluster" || echo "FAIL: Default expected default-error-pages-hive2040, got $DEFAULT_SPOKE_CONFIG"

   # Verify custom IngressController
   CUSTOM_SPOKE_CONFIG=$(oc --kubeconfig=${SPOKE_KUBECONFIG} get ingresscontroller custom -n openshift-ingress-operator -o jsonpath='{.spec.httpErrorCodePages.name}')
   [[ "$CUSTOM_SPOKE_CONFIG" == "custom-error-pages-hive2040" ]] && echo "PASS: Custom IngressController correctly configured in spoke cluster" || echo "FAIL: Custom expected custom-error-pages-hive2040, got $CUSTOM_SPOKE_CONFIG"
   ```
   **Expected:** Both IngressController resources in spoke cluster have correct respective HttpErrorCodePages configurations, validations show "PASS" messages

### Test Case HIVE-2040_004
**Name:** Verify HttpErrorCodePages configuration update propagation
**Description:** Validate that updates to HttpErrorCodePages configuration in ClusterDeployment.Spec.Ingress[] correctly propagate to the spoke cluster IngressController
**Type:** Update Scenario
**Priority:** Medium

#### Prerequisites
- Hive management cluster with RemoteClusterIngress controller running
- Target spoke cluster accessible and healthy
- Existing ClusterDeployment from Test Case HIVE-2040_001 or create new one
- Two different ConfigMaps for testing update:
```yaml
# Original ConfigMap
apiVersion: v1
kind: ConfigMap
metadata:
  name: original-error-pages-hive2040
  namespace: openshift-config
data:
  error-page-503.http: |
    <html><body><h1>Original Service Unavailable</h1></body></html>
---
# Updated ConfigMap
apiVersion: v1
kind: ConfigMap
metadata:
  name: updated-error-pages-hive2040
  namespace: openshift-config
data:
  error-page-503.http: |
    <html><body><h1>Updated Service Unavailable</h1></body></html>
```
- Cluster admin access to both management and spoke clusters

#### Test Steps
1. **Action:** Update ClusterDeployment to change HttpErrorCodePages configuration
   ```bash
   # First verify current configuration
   CURRENT_CONFIG=$(oc get clusterdeployment test-cluster-hive2040 -n hive-test -o jsonpath='{.spec.ingress[0].httpErrorCodePages.name}')
   echo "Current HttpErrorCodePages: $CURRENT_CONFIG"

   # Update the HttpErrorCodePages configuration
   oc patch clusterdeployment test-cluster-hive2040 -n hive-test --type='merge' -p='{"spec":{"ingress":[{"name":"default","domain":"*.apps.test-hive2040.example.com","httpErrorCodePages":{"name":"updated-error-pages-hive2040"}}]}}'

   # Verify patch applied
   UPDATED_CONFIG=$(oc get clusterdeployment test-cluster-hive2040 -n hive-test -o jsonpath='{.spec.ingress[0].httpErrorCodePages.name}')
   [[ "$UPDATED_CONFIG" == "updated-error-pages-hive2040" ]] && echo "PASS: ClusterDeployment updated successfully" || echo "FAIL: Update failed, got $UPDATED_CONFIG"
   ```
   **Expected:** ClusterDeployment patch succeeds, HttpErrorCodePages changes to "updated-error-pages-hive2040", validation shows "PASS"

2. **Action:** Verify SyncSet update contains new IngressController configuration
   ```bash
   # Wait for SyncSet reconciliation (up to 120 seconds)
   for i in {1..24}; do
     sleep 5
     SYNCSET_CONFIG=$(oc get syncset test-cluster-hive2040-clusteringress -n hive-test -o jsonpath='{.spec.resources[0].object.spec.httpErrorCodePages.name}')
     if [[ "$SYNCSET_CONFIG" == "updated-error-pages-hive2040" ]]; then
       echo "PASS: SyncSet updated after ${i}*5 seconds"
       break
     elif [[ $i -eq 24 ]]; then
       echo "FAIL: SyncSet not updated within 120 seconds, current value: $SYNCSET_CONFIG"
       exit 1
     fi
   done
   ```
   **Expected:** SyncSet updates within 120 seconds to contain "updated-error-pages-hive2040", validation shows "PASS"

3. **Action:** Validate updated configuration propagates to spoke cluster IngressController
   ```bash
   # Monitor spoke cluster IngressController for configuration update (up to 180 seconds)
   for i in {1..36}; do
     sleep 5
     SPOKE_CONFIG=$(oc --kubeconfig=${SPOKE_KUBECONFIG} get ingresscontroller default -n openshift-ingress-operator -o jsonpath='{.spec.httpErrorCodePages.name}')
     if [[ "$SPOKE_CONFIG" == "updated-error-pages-hive2040" ]]; then
       echo "PASS: Spoke cluster IngressController updated after ${i}*5 seconds"
       break
     elif [[ $i -eq 36 ]]; then
       echo "FAIL: Spoke cluster not updated within 180 seconds, current value: $SPOKE_CONFIG"
       exit 1
     fi
   done

   # Verify configuration remains stable
   sleep 30
   FINAL_CONFIG=$(oc --kubeconfig=${SPOKE_KUBECONFIG} get ingresscontroller default -n openshift-ingress-operator -o jsonpath='{.spec.httpErrorCodePages.name}')
   [[ "$FINAL_CONFIG" == "updated-error-pages-hive2040" ]] && echo "PASS: Configuration stable after update" || echo "FAIL: Configuration unstable: $FINAL_CONFIG"
   ```
   **Expected:** IngressController in spoke cluster updates to "updated-error-pages-hive2040" within 180 seconds, remains stable for 30 seconds, validations show "PASS" messages