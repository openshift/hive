# Test Case: HIVE-2923
**Component:** hive
**Summary:** Scrub AWS RequestID from DNSZone "DNSError" status condition message (thrashing)

## Test Overview
- **Total Test Cases:** 2
- **Test Types:** Error Handling, Message Scrubbing Validation
- **Estimated Time:** 30 minutes

## Test Cases

### Test Case HIVE-2923_001
**Name:** Verify AWS RequestID scrubbing in DNSZone error messages with invalid credentials
**Description:** Validate that AWS RequestID (UUID) is properly scrubbed from DNSZone status condition messages to prevent reconciliation thrashing
**Type:** Error Handling
**Priority:** High

#### Prerequisites
- OpenShift cluster with Hive installed
- Invalid AWS credentials configured in a Secret
- Access to oc CLI with cluster-admin permissions

#### Test Steps
1. **Action:** Create a ClusterDeployment with manageDNS=true using invalid AWS credentials
   ```bash
   # Create invalid AWS credentials secret
   oc create secret generic invalid-aws-creds --from-literal=aws_access_key_id=INVALID_KEY --from-literal=aws_secret_access_key=INVALID_SECRET -n default

   # Create ClusterDeployment with manageDNS enabled
   cat <<EOF | oc apply -f -
   apiVersion: hive.openshift.io/v1
   kind: ClusterDeployment
   metadata:
     name: test-dns-scrub-cd
     namespace: default
   spec:
     clusterName: test-dns-scrub
     platform:
       aws:
         credentialsSecretRef:
           name: invalid-aws-creds
         region: us-east-1
     manageDNS: true
     baseDomain: example.com
   EOF
   ```
   **Expected:** ClusterDeployment created and DNSZone resource automatically generated

2. **Action:** Wait for DNSZone to encounter AWS API errors and capture initial status message
   ```bash
   # Wait for DNSZone creation and error condition
   oc wait --for=condition=Ready=false dnszone --all --timeout=300s

   # Capture initial error message
   INITIAL_MESSAGE=$(oc get dnszone -o jsonpath='{.items[0].status.conditions[?(@.type=="DNSError")].message}')
   echo "Initial error message: $INITIAL_MESSAGE"
   ```
   **Expected:** DNSZone shows DNSError condition with AWS API error message, no RequestID pattern visible

3. **Action:** Monitor DNSZone status condition message stability across multiple reconciliation cycles
   ```bash
   # Monitor message stability for 5 minutes (15 reconciliation cycles)
   for i in {1..15}; do
     sleep 20
     CURRENT_MESSAGE=$(oc get dnszone -o jsonpath='{.items[0].status.conditions[?(@.type=="DNSError")].message}')
     echo "Cycle $i message: $CURRENT_MESSAGE"

     # Verify message remains identical to initial
     if [ "$CURRENT_MESSAGE" != "$INITIAL_MESSAGE" ]; then
       echo "ERROR: Message changed in cycle $i"
       echo "Expected: $INITIAL_MESSAGE"
       echo "Actual: $CURRENT_MESSAGE"
       exit 1
     fi
   done
   echo "SUCCESS: Message remained stable across 15 reconciliation cycles"
   ```
   **Expected:** DNSZone error message remains identical across all reconciliation cycles, proving no RequestID thrashing

4. **Action:** Validate that AWS RequestID pattern is absent from error message
   ```bash
   # Check for RequestID patterns in error message
   ERROR_MESSAGE=$(oc get dnszone -o jsonpath='{.items[0].status.conditions[?(@.type=="DNSError")].message}')

   # Search for UUID patterns (AWS RequestID format)
   UUID_PATTERN="[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}"
   if echo "$ERROR_MESSAGE" | grep -iE "$UUID_PATTERN"; then
     echo "FAIL: RequestID/UUID found in error message: $ERROR_MESSAGE"
     exit 1
   else
     echo "PASS: No RequestID/UUID pattern found in error message"
   fi

   # Search for "request" + "id" patterns
   if echo "$ERROR_MESSAGE" | grep -i "request.*id"; then
     echo "FAIL: RequestID reference found in error message: $ERROR_MESSAGE"
     exit 1
   else
     echo "PASS: No RequestID reference found in error message"
   fi
   ```
   **Expected:** No UUID patterns or RequestID references found in error message, confirming proper scrubbing

### Test Case HIVE-2923_002
**Name:** Verify DNSZone reconciliation frequency with invalid credentials
**Description:** Validate that DNSZone reconciliation does not exhibit excessive frequency due to RequestID thrashing
**Type:** Performance Validation
**Priority:** High

#### Prerequisites
- Test Case HIVE-2923_001 completed successfully
- DNSZone with DNSError condition already present

#### Test Steps
1. **Action:** Monitor DNSZone reconciliation frequency over time window
   ```bash
   # Get initial resourceVersion and generation
   INITIAL_RV=$(oc get dnszone -o jsonpath='{.items[0].metadata.resourceVersion}')
   INITIAL_GEN=$(oc get dnszone -o jsonpath='{.items[0].metadata.generation}')
   echo "Initial ResourceVersion: $INITIAL_RV, Generation: $INITIAL_GEN"

   # Monitor for 10 minutes to count status updates
   START_TIME=$(date +%s)
   UPDATE_COUNT=0

   while [ $(($(date +%s) - START_TIME)) -lt 600 ]; do
     sleep 10
     CURRENT_RV=$(oc get dnszone -o jsonpath='{.items[0].metadata.resourceVersion}')

     if [ "$CURRENT_RV" != "$INITIAL_RV" ]; then
       UPDATE_COUNT=$((UPDATE_COUNT + 1))
       echo "Update #$UPDATE_COUNT at $(date): ResourceVersion changed from $INITIAL_RV to $CURRENT_RV"
       INITIAL_RV=$CURRENT_RV
     fi
   done

   echo "Total status updates in 10 minutes: $UPDATE_COUNT"
   ```
   **Expected:** DNSZone status updates should be ≤5 times in 10 minutes (normal backoff behavior), not continuous thrashing

2. **Action:** Verify successful recovery when valid credentials are provided
   ```bash
   # Replace with valid AWS credentials (use test environment credentials)
   oc patch secret invalid-aws-creds --type='json' -p='[
     {"op": "replace", "path": "/data/aws_access_key_id", "value": "'$(echo -n "$VALID_AWS_KEY" | base64)'"},
     {"op": "replace", "path": "/data/aws_secret_access_key", "value": "'$(echo -n "$VALID_AWS_SECRET" | base64)'"}
   ]'

   # Wait for DNSZone to recover
   oc wait --for=condition=Ready=true dnszone --all --timeout=300s

   # Verify error condition cleared
   ERROR_COUNT=$(oc get dnszone -o jsonpath='{.items[0].status.conditions[?(@.type=="DNSError")]}' | wc -l)
   if [ "$ERROR_COUNT" -eq 0 ]; then
     echo "SUCCESS: DNSError condition cleared after credential fix"
   else
     echo "FAIL: DNSError condition still present after credential fix"
     exit 1
   fi
   ```
   **Expected:** DNSZone transitions from error state to healthy state within 5 minutes, DNSError condition cleared

---
*Generated from Markdown template*