# E2E Test Case Addition Guidelines for Hive

## 📋 Overview & Summary  
This guideline standardizes the addition and management of Hive E2E test cases, ensuring consistent structure, clear classification, and maintainable and executable tests.

### 🏗️ Architecture Overview
Hive E2E tests follow a **platform-separated architecture** with dedicated files for each cloud provider. Each platform file includes comprehensive tests for cluster lifecycle management, while `hive_util.go` provides shared utilities.

### 🚨 CRITICAL SUCCESS FACTORS:
1. **Follow Existing Patterns**: Always learn from and follow existing test code patterns
2. **Use Correct Platform Files**: Add tests to appropriate platform-specific files
3. **Follow Naming Conventions**: Use established naming conventions and templates
4. **Include Proper Cleanup**: Use defer statements and proper error handling

**The key is to use the correct platform file, follow established naming conventions, and leverage existing templates and utility functions for consistent, maintainable tests.**

---

## 🚨 MANDATORY STEPS

### 🔍 CRITICAL RULE: Understand Test File Structure and Organization
**BEFORE adding any test case, you MUST understand the target test file structure, test case organization, and platform-specific configurations.**

### ⚠️ CRITICAL RULE: Do Not Create New Test Files
**NEVER create new test files for Hive E2E tests.** All test cases must be added to existing platform-specific files based on the target platform.

---

## 📋 Test File Organization Rules

### 🏷️ Rule 0: RFC 1123 Naming Compliance
**CRITICAL: All test case IDs must comply with RFC 1123 naming standards for Kubernetes resources.**

**Conversion Rules:**
- Convert JIRA key to lowercase
- Remove all symbols (dashes, underscores, etc.)
- Example: `HIVE-2544` becomes `testCaseID := "hive2544"`

### Rule 1: Mandatory Steps Integration
**MANDATORY: Always follow patterns from existing test code.**

### Rule 2: Determine the Target File
**MANDATORY: Identify the correct platform-specific test file based on the target platform.**

**Platform File Mapping:**
- **AWS**: `hive_aws.go`
- **Azure**: `hive_azure.go` 
- **GCP**: `hive_gcp.go`
- **vSphere**: `hive_vsphere.go`
- **Bare Metal**: `hive_baremetal.go`

**File Selection Criteria:**
1. **Primary Platform**: Use the main cloud provider for the test
2. **Cross-Platform Tests**: Choose the most appropriate file or create platform-agnostic tests
3. **Existing Patterns**: Follow how similar tests are organized in existing files

### Rule 3: Test Case Naming Convention
**MANDATORY: Follow established naming conventions for test cases.**

**Naming Format:**
```go
g.It("NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:{AUTHOR}-Medium-{JIRA_KEY}-{Test Description} [Serial]", func() {
```

**Naming Components:**
- **NonHyperShiftHOST**: Standard prefix for Hive tests
- **Longduration/NonPreRelease**: Test duration and release type
- **ConnectedOnly**: Network connectivity requirement
- **Author**: Test author name
- **Medium**: Test priority level
- **JIRA_KEY**: Corresponding JIRA ticket (e.g., HIVE-2544)
- **Test Description**: Clear, descriptive test name
- **[Serial]**: Serial execution requirement

**Examples:**
```go
// ClusterDeployment test
g.It("NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:mihuang-Medium-35297-Hive supports cluster hibernation[Serial]", func() {

// MachinePool test
g.It("NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:lwan-Medium-52415-[Azure]Hive Machinepool test for autoscale [Serial]", func() {

// Recent Azure Government test
g.It("NonHyperShiftHOST-NonPreRelease-Author:mihuang-Medium-HIVE-2544-Machinepool functionality in Azure regions without availability zone support [Serial]", func() {
```

### Rule 4: Platform-Specific Setup Patterns
**MANDATORY: Use platform-specific setup patterns and configurations.**

**Platform Detection:**
```go
// MANDATORY: Include platform detection
exutil.SkipIfPlatformTypeNot("AWS", "Azure", "GCP", "vSphere")
```

**Platform-Specific Configurations:**
- **AWS**: Use AWS-specific regions, instance types, and configurations
- **Azure**: Use Azure-specific regions, VM sizes, and configurations  
- **GCP**: Use GCP-specific regions, machine types, and configurations
- **vSphere**: Use vSphere-specific datacenter and resource configurations

---

## 🌐 Platform-Specific Checks

### Azure Platform Checks
**If the test requires Azure Government Cloud, you MUST first check if the current test environment is Azure Gov Cloud using skip logic:**

```go
// Check if test requires Azure Government Cloud
if !isGovCloud {
    g.Skip("Test requires Azure Government Cloud environment, current region: " + region)
}
```

**If the test works on both clouds (platform-agnostic), no platform check is needed.**

### AWS Platform Checks
**If the test requires AWS Government Cloud, you MUST first check if the current test environment is AWS Gov Cloud using skip logic:**

```go
// Check if test requires AWS Government Cloud
if !isGovCloud {
    g.Skip("Test requires AWS Government Cloud environment, current region: " + region)
}
```

**If the test works on both AWS clouds (platform-agnostic), no platform check is needed.**

---

## 🔍 Template Strategy: Field Compatibility Analysis

### 🚨 CRITICAL: CRD Field Compatibility Checking

**MANDATORY: When tests involve new fields in existing resources (ClusterDeployment, MachinePool, etc.), you MUST check field compatibility before deciding template strategy.**

#### Decision Process for Template Strategy

**Step 1: Check New Field CRD Definition**
```bash
# Query the CRD definition for the new field
# Example for HttpErrorCodePages in ClusterDeployment:
oc get crd clusterdeployments.hive.openshift.io -o yaml | grep -A 5 -B 5 httpErrorCodePages
```

**Step 2: Analyze Field Characteristics**
Look for these indicators:
- **Pointer types**: `*configv1.ConfigMapNameReference` (optional field)
- **omitempty tags**: `,omitempty` (field can be omitted)
- **Optional annotations**: `// +optional` (field is optional)
- **Required validation**: `// +kubebuilder:validation:Required` (field is mandatory)

**Step 3: Template Strategy Decision Matrix**

| Field Characteristics | Template Strategy | Action |
|----------------------|-------------------|---------|
| **Optional + omitempty** | **Merge to existing template** | Add field to existing template with proper conditional logic |
| **Required field** | **Create new template** | Create separate template file for compatibility |
| **Breaking changes** | **Create new template** | Create separate template to avoid affecting existing tests |
| **Backward compatible** | **Merge to existing template** | Safe to add to existing template |

#### Example: HIVE-2040 HttpErrorCodePages Analysis

**Field Definition Found:**
```go
// HttpErrorCodePages references a ConfigMap with custom error page content
// +optional
HttpErrorCodePages *configv1.ConfigMapNameReference `json:"httpErrorCodePages,omitempty"`
```

**Analysis Result:**
- ✅ **Pointer type**: `*configv1.ConfigMapNameReference` (optional)
- ✅ **omitempty tag**: `,omitempty` (can be omitted)
- ✅ **Optional annotation**: `// +optional` (explicitly optional)

**Decision**: **Merge to existing template** ✅
- Field is fully backward compatible
- Existing tests will not be affected
- Can use patch approach instead of template modification

#### Template Strategy Implementation

**Option 1: Merge to Existing Template (Recommended for optional fields)**
```go
// Use JSON patch to add new field to existing ClusterDeployment
patchData := `[{
    "op": "add",
    "path": "/spec/ingress",
    "value": [
        {
            "name": "default",
            "domain": "apps.` + cdName + `.example.com",
            "httpErrorCodePages": {
                "name": "` + configMapName + `"
            }
        }
    ]
}]`
```

**Option 2: Create New Template (For required/breaking fields)**
```yaml
# Create new template file: clusterdeployment-with-new-field.yaml
apiVersion: template.openshift.io/v1
kind: Template
metadata:
  name: clusterdeployment-new-field
parameters:
- name: NEW_FIELD_VALUE
  required: true
objects:
- apiVersion: hive.openshift.io/v1
  kind: ClusterDeployment
  spec:
    newRequiredField: "${NEW_FIELD_VALUE}"
```

### 🔧 Implementation Guidelines

**For Optional Fields (Recommended Approach):**
1. **Use existing templates unchanged**
2. **Apply JSON patches for new field configuration**
3. **Test both configured and non-configured scenarios**
4. **Ensure backward compatibility**

**For Required/Breaking Fields:**
1. **Create new template file**
2. **Follow existing template patterns**
3. **Add to template parameter list**
4. **Use template for test creation**

**Validation Steps:**
1. **CRD Analysis**: Check field definition and characteristics
2. **Compatibility Test**: Verify existing tests are not affected
3. **Integration Test**: Test both old and new field scenarios
4. **Documentation**: Document template strategy decision

---

## 📋 ClusterDeployment Testing

### 🔍 MANDATORY: ClusterDeployment Test Structure

**Use this template structure for all ClusterDeployment tests:**

```go
g.It("NonHyperShiftHOST-NonPreRelease-Author:mihuang-Medium-HIVE-XXXX-ClusterDeployment functionality description [Serial]", func() {
    // CRITICAL: Convert JIRA key to lowercase and remove symbols for RFC 1123 compliance
    testCaseID := "hiveXXXX"  // Convert HIVE-XXXX to lowercase without symbols
    cdName := "cluster-" + testCaseID + "-" + getRandomString()[:ClusterSuffixLen]
    
    exutil.By("Creating ClusterDeployment")
    // Use platform-specific cluster deployment template
    
    exutil.By("Waiting for cluster installation")
    // Use newCheck() patterns for validation
    
    exutil.By("Testing specific functionality")
    // Implement test-specific logic
    
    exutil.By("Cleanup")
    // Use defer for cleanup operations
})
```

### 🚨 CRITICAL: ClusterDeployment Test Requirements

**ClusterDeployment tests MUST include:**

1. **Template Usage**: Always use existing ClusterDeployment templates
2. **Wait Conditions**: Wait for cluster installation completion
3. **Status Verification**: Verify cluster installation status
4. **Proper Cleanup**: Use defer statements for cleanup
5. **Error Handling**: Proper error handling for all operations
6. **Timeout Management**: Appropriate timeouts for cluster operations

---

## 📋 Machinepool Testing

### 🔍 MANDATORY: MachinePool Test Structure

**For all MachinePool tests, reference the existing e2e test with `testCaseID := "25447"`:**
- **Reference**: Use the existing e2e test with `testCaseID := "25447"`
- **Test Case ID**: `testCaseID := "25447"`
- **Follow the exact structure and validation steps from the existing e2e test**  

**Use this template structure for all MachinePool tests:**

```go
g.It("NonHyperShiftHOST-NonPreRelease-Author:mihuang-Medium-HIVE-XXXX-Machinepool functionality description [Serial]", func() {
    // CRITICAL: Convert JIRA key to lowercase and remove symbols for RFC 1123 compliance
    testCaseID := "hiveXXXX"  // Convert HIVE-XXXX to lowercase without symbols
    cdName := "cluster-" + testCaseID + "-" + getRandomString()[:ClusterSuffixLen]
    
    exutil.By("Creating ClusterDeployment")
    // Use platform-specific cluster deployment template

    exutil.By("Creating MachinePool")
    // Use platform-specific machinepool template
    
    exutil.By("Waiting for cluster installation")
    // Wait for cluster to be installed
    
    exutil.By("Waiting for MachinePool to be ready")
    // Wait for MachinePool to be ready

    exutil.By("Testing specific functionality")
    // Implement test-specific logic
    
    exutil.By("Cleanup")
    // Cleanup is handled by defer statements
})
```
### 🔍 MachinePool Creation in Non-AZ Regions

**If your test is running in Azure regions without availability zone support (e.g., usgovtexas), you can reference the JSONPath Validation Commands below for validation patterns:**

#### Expected Behavior for Non-AZ Regions
```go
// If testing in regions like usgovtexas (Azure Government) without zone support:
// 1. Only ONE worker machineset should be created (not multiple zone-specific ones)
// 2. MachineSet name format: {infraID}-{poolName}-{region} (no zone suffix)
// 3. No zone field in providerSpec or zone field is empty
// 4. All replicas distributed in single machineset
```

#### JSONPath Validation Commands
```go
// 1. Get all MachineSet names using oc command (recommended approach)
machineSetNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("--kubeconfig="+kubeconfig, "MachineSet", "-n", "openshift-machine-api", "-l", "hive.openshift.io/machine-pool=worker", "-o=jsonpath={.items[*].metadata.name}").Output()
o.Expect(err).NotTo(o.HaveOccurred())

// 2. For non-AZ regions, should have exactly one MachineSet name (no spaces = single item)
machineSetNameList := strings.Fields(machineSetNames)
o.Expect(len(machineSetNameList)).To(o.Equal(1), "Expected exactly one MachineSet for non-AZ region, got: %v", machineSetNameList)

// 3. Verify replicas in machineset (example: 3 3 3)
newCheck("expect", "get", asAdmin, withoutNamespace, compare, "3 3 3", ok, 5*DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", "-n", "openshift-machine-api", "-l", "hive.openshift.io/machine-pool=worker", "-o=jsonpath={.items[0].spec.replicas} {.items[0].status.readyReplicas} {.items[0].status.availableReplicas}"}).check(oc)
```

#### Key Differences from Zone-Supported Regions
**Zone-Supported Regions:**
- Multiple machinesets (one per zone): `infraID-poolName-region-1`, `infraID-poolName-region-2`
- Zone field populated: `zone: "1"`, `zone: "2"`, `zone: "3"`
- Replicas distributed across multiple machinesets

**Non-AZ Regions (e.g., usgovtexas):**
- Single machineset: `infraID-poolName-usgovtexas`
- Zone field empty or missing: `zone: ""`
- All replicas in one machineset: `replicas: 3`
- Machines may still show zone 0 or 1 (Azure internal assignment)

#### Optional: Autoscaling Testing (Add Only If Required)
**If your test case requires autoscaling functionality, add these additional steps:**

```go
// OPTIONAL: Only add if test case specifically requires autoscaling validation

exutil.By("Creating MachinePool with autoscaling")
// Configure MachinePool with minReplicas and maxReplicas
// Example: minReplicas: 1, maxReplicas: 5

exutil.By("Testing autoscaling scale-up")
// Create workload to trigger scaling (e.g., busybox pods)
// Wait for cluster autoscaler to scale up

exutil.By("Testing autoscaling scale-down")
// Remove workload to trigger scaling down
// Wait for cluster autoscaler to scale down to minReplicas
```

### 🚨 CRITICAL: MachinePool Test Requirements

**MachinePool tests MUST include:**

1. **Pre-requisite Validation**: Cluster must be installed and ready
2. **Template Usage**: Always use existing MachinePool templates
3. **Status Verification**: Wait for MachinePool to be ready
4. **Scaling Tests**: If testing scaling functionality
5. **Node Verification**: Verify nodes are actually created in target cluster
6. **Proper Cleanup**: Use defer statements for cleanup
7. **Error Handling**: Proper error handling for all operations
8. **Timeout Management**: Appropriate timeouts for cloud operations

### 🚨 CRITICAL: MachinePool vs ClusterDeployment Testing

**Testing Strategy:**
<!-- - **ClusterDeployment (CD) tests**: Can use fake clusters for basic functionality testing -->
- **MachinePool tests**: MUST use real cloud resources (cannot use fake)

**Why MachinePool requires real resources:**
- MachinePool needs actual cloud provider APIs for node provisioning
- Real cloud infrastructure required for scaling operations
- Actual cloud networking needed for node communication
- Real cloud storage required for node persistence

---

## 📋 ClusterPool Testing

### 🔍 MANDATORY: ClusterPool Test Structure

**Use this template structure for all ClusterPool tests:**

```go
g.It("NonHyperShiftHOST-NonPreRelease-Author:mihuang-Medium-HIVE-XXXX-ClusterPool functionality description [Serial]", func() {
    // CRITICAL: Convert JIRA key to lowercase and remove symbols for RFC 1123 compliance
    testCaseID := "hiveXXXX"  // Convert HIVE-XXXX to lowercase without symbols
    poolName := "pool-" + testCaseID + "-" + getRandomString()[:ClusterSuffixLen]
    
    exutil.By("Creating ClusterPool")
    // Use platform-specific cluster pool template
    
    exutil.By("Waiting for ClusterPool to be ready")
    // Wait for ClusterPool to be ready
    
    exutil.By("Testing specific functionality")
    // Implement test-specific logic
    
    exutil.By("Cleanup")
    // Use defer for cleanup operations
})
```

### 🚨 CRITICAL: ClusterPool Test Requirements

**ClusterPool tests MUST include:**

1. **Template Usage**: Always use existing ClusterPool templates
2. **Wait Conditions**: Wait for ClusterPool to be ready
3. **Status Verification**: Verify ClusterPool status and conditions
4. **Proper Cleanup**: Use defer statements for cleanup
5. **Error Handling**: Proper error handling for all operations
6. **Timeout Management**: Appropriate timeouts for pool operations

---

## 🚫 Forbidden Actions & Correct Practices

### Forbidden Actions:
- Creating new test files (use existing platform files)
- Modifying shared utility functions in `hive_util.go` without coordination
- Adding tests without proper platform detection
- Skipping architecture checks
- Missing cleanup operations
- **Generating tests without analyzing existing patterns from existing test code**
- **Ignoring test requirements analysis and functional understanding**
- **Using generic patterns instead of learning from existing Hive test code**
- **Creating inline YAML for MachinePool resources (use template files instead)**
- **Using ioutil.WriteFile for temporary YAML files (use machinepool struct and templates)**
<!-- - **CRITICAL: Using fake clusters for MachinePool testing (MachinePool requires real cloud resources, but ClusterDeployment can use fake)** -->
### Correct Practices:
- **ALWAYS analyze test requirements and functional understanding first**
- **ALWAYS follow patterns from existing test code**
- **ALWAYS learn from existing test code in target platform file**
- Add tests to appropriate platform-specific file
- Use existing utility functions from `hive_util.go`
- Include proper platform detection with `exutil.SkipIfPlatformTypeNot()`
- Follow established naming conventions
- Use templates from `testdata/cluster_operator/hive/`
- Include proper cleanup with `defer` statements
- Use `exutil.By()` for step organization
- **Apply learned patterns consistently with existing code**
- **ALWAYS use template files for MachinePool creation via machinepool struct**
- **NEVER create inline YAML strings for MachinePool resources**

---

## 🌍 Testing Region Configuration

### 🌍 MANDATORY: Testing Region Replacement

**If the test requires using a testing region instead of the current environment region, you MUST replace the region variable:**

```go
// Option 1: Use specific testing region
spokeRegion := "usgovtexas"     // For Azure Gov Cloud testing
spokeRegion := "eastus"         // For Azure Public Cloud testing
spokeRegion := "us-east-1"      // For AWS testing

// If not specified, use default current environment region
```

### Testing Region Decision Logic:
- **Specific Test Requirements**: Use the region specified in JIRA requirements
- **Testing Constants**: Use predefined testing region constants when available
- **Default Behavior**: If not specified, use current environment region
- **Platform-Specific**: Choose region based on platform requirements (Gov Cloud vs Public Cloud)

---

## 🚨 CRITICAL: No Redundant Logging

**CRITICAL: Avoid redundant logging in test code. Only include essential logs for debugging and validation.**

**Examples of redundant logging to avoid:**
```go
// ❌ REDUNDANT - Don't log obvious operations
fmt.Printf("Starting test execution\n")
fmt.Printf("Creating cluster deployment\n")
fmt.Printf("Waiting for cluster to be ready\n")

// ✅ CORRECT - Only log important validation points
exutil.By("Verifying cluster installation status")
exutil.By("Checking MachinePool readiness")
```

**Logging Best Practices:**
- Use `exutil.By()` for test step organization
- Only log critical validation points
- Avoid logging obvious operations
- Focus on meaningful test progress indicators

---

## 🔍 Template Strategy: Field Compatibility Analysis

### 🚨 CRITICAL: CRD Field Compatibility Checking

**MANDATORY: When tests involve new fields in existing resources (ClusterDeployment, MachinePool, etc.), you MUST check field compatibility before deciding template strategy.**

Check whether new fields can be added to existing templates by examining the CRD definition of the new field. If the new field supports being empty, merge the new field into the existing template. If the new field is incompatible with the existing template, create a new template.

#### Decision Process for Template Strategy

**Step 1: Check New Field CRD Definition**
```bash
# Query the CRD definition for the new field
# Example for HttpErrorCodePages in ClusterDeployment:
oc get crd clusterdeployments.hive.openshift.io -o yaml | grep -A 5 -B 5 httpErrorCodePages
```

**Step 2: Analyze Field Characteristics**
Look for these indicators:
- **Pointer types**: `*configv1.ConfigMapNameReference` (optional field)
- **omitempty tags**: `,omitempty` (field can be omitted)
- **Optional annotations**: `// +optional` (field is optional)
- **Required validation**: `// +kubebuilder:validation:Required` (field is mandatory)

**Step 3: Template Strategy Decision Matrix**

| Field Characteristics | Template Strategy | Action |
|----------------------|-------------------|---------|
| **Optional + omitempty** | **Merge to existing template** | Add field to existing template with proper conditional logic |
| **Required field** | **Create new template** | Create separate template file for compatibility |
| **Breaking changes** | **Create new template** | Create separate template to avoid affecting existing tests |
| **Backward compatible** | **Merge to existing template** | Safe to add to existing template |

#### Example: HIVE-2040 HttpErrorCodePages Analysis

**Field Definition Found:**
```go
// HttpErrorCodePages references a ConfigMap with custom error page content
// +optional
HttpErrorCodePages *configv1.ConfigMapNameReference `json:"httpErrorCodePages,omitempty"`
```

**Analysis Result:**
- ✅ **Pointer type**: `*configv1.ConfigMapNameReference` (optional)
- ✅ **omitempty tag**: `,omitempty` (can be omitted)
- ✅ **Optional annotation**: `// +optional` (explicitly optional)

**Decision**: **Merge to existing template** ✅
- Field is fully backward compatible
- Existing tests will not be affected
- Can use patch approach instead of template modification

#### Implementation Guidelines

**For Optional Fields (Recommended Approach):**
1. **Use existing templates unchanged**
2. **Apply JSON patches for new field configuration**
3. **Test both configured and non-configured scenarios**
4. **Ensure backward compatibility**

**For Required/Breaking Fields:**
1. **Create new template file**
2. **Follow existing template patterns**
3. **Add to template parameter list**
4. **Use template for test creation**
