# Test Case: HIVE-2040
**Component:** hive  
**Summary:** Add HttpErrorCodePages to CD.Spec.Ingress[]

## Test Overview
- **Total Test Cases:** 2
- **Test Types:** Functional, Integration
- **Estimated Time:** 60 minutes

## Test Cases

### Test Case HIVE-2040_001
**Name:** Verify HttpErrorCodePages field propagates to IngressController when configured  
**Description:** Test that HttpErrorCodePages field in ClusterDeployment.Spec.Ingress[] is correctly propagated to the default IngressController in openshift-ingress-operator namespace  
**Type:** Functional  
**Priority:** High

#### Prerequisites
- Hive operator installed and running
- Target OpenShift cluster available for ClusterDeployment
- Access to openshift-ingress-operator namespace in target cluster
- Ability to create ConfigMaps in target cluster

#### Test Steps  
1. **Action:** Create a ClusterDeployment with HttpErrorCodePages field configured in spec.ingress[]
   ```yaml
   apiVersion: hive.openshift.io/v1
   kind: ClusterDeployment
   metadata:
     name: test-cluster-httperror
   spec:
     ingress:
     - name: default
       domain: apps.test-cluster.example.com
       httpErrorCodePages:
         name: custom-error-pages
   ```      
   **Expected:** ClusterDeployment is created successfully and begins provisioning

2. **Action:** Create a ConfigMap with custom error pages in the target cluster's openshift-config namespace
   ```yaml
   apiVersion: v1
   kind: ConfigMap
   metadata:
     name: custom-error-pages
     namespace: openshift-config
   data:
     404.html: |
       <html><body><h1>Custom 404 Error</h1></body></html>
     503.html: |
       <html><body><h1>Custom 503 Error</h1></body></html>
   ```      
   **Expected:** ConfigMap is created successfully in openshift-config namespace

3. **Action:** Wait for cluster provisioning to complete and verify the IngressController in the target cluster
   ```bash
   oc get ingresscontroller default -n openshift-ingress-operator -o yaml
   ```      
   **Expected:** IngressController contains httpErrorCodePages field with name "custom-error-pages"

4. **Action:** Verify the httpErrorCodePages field is correctly set in the IngressController spec
   ```bash
   oc get ingresscontroller default -n openshift-ingress-operator -o jsonpath='{.spec.httpErrorCodePages.name}'
   ```      
   **Expected:** Output shows "custom-error-pages"

### Test Case HIVE-2040_002  
**Name:** Verify IngressController has empty HttpErrorCodePages when not configured    
**Description:** Test that when HttpErrorCodePages field is not specified in ClusterDeployment.Spec.Ingress[], the default IngressController has empty/missing HttpErrorCodePages field    
**Type:** Functional    
**Priority:** Medium  
 
#### Prerequisites  
- Hive operator installed and running
- Target OpenShift cluster available for ClusterDeployment
- Access to openshift-ingress-operator namespace in target cluster

#### Test Steps  
1. **Action:** Create a ClusterDeployment without HttpErrorCodePages field in spec.ingress[]
   ```yaml
   apiVersion: hive.openshift.io/v1
   kind: ClusterDeployment
   metadata:
     name: test-cluster-no-httperror
   spec:
     ingress:
     - name: default
       domain: apps.test-cluster-basic.example.com
   ```      
   **Expected:** ClusterDeployment is created successfully and begins provisioning

2. **Action:** Wait for cluster provisioning to complete and check the IngressController in the target cluster
   ```bash
   oc get ingresscontroller default -n openshift-ingress-operator -o yaml
   ```      
   **Expected:** IngressController does not contain httpErrorCodePages field or field is empty/null

3. **Action:** Verify the httpErrorCodePages field is not present or empty in the IngressController spec
   ```bash
   oc get ingresscontroller default -n openshift-ingress-operator -o jsonpath='{.spec.httpErrorCodePages.name}' || echo "Field not present"
   ```      
   **Expected:** Command returns empty output or "Field not present" message

4. **Action:** Inspect the complete IngressController spec to confirm default behavior
   ```bash
   oc get ingresscontroller default -n openshift-ingress-operator -o jsonpath='{.spec}' | jq '.httpErrorCodePages // "not_configured"'
   ```      
   **Expected:** Output shows "not_configured" or null, confirming field is not set

---
*Generated from Markdown template*