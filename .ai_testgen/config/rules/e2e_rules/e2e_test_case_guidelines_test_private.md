# E2E Test Case Addition Guidelines for Hive

---

## Overview
This guideline standardizes the addition and management of Hive E2E test cases, ensuring consistent structure, clear classification, and maintainable and executable tests.  
All new E2E test cases must follow the rules and steps outlined below.

---

## ⚠️ CRITICAL RULE: Do Not Create New Test Files

**NEVER create new test files for Hive E2E tests.** All test cases must be added to existing platform-specific files based on the target platform.

### Platform-Based File Assignment:
- **AWS Tests** → Add to `hive_aws.go`
- **Azure Tests** → Add to `hive_azure.go` 
- **GCP Tests** → Add to `hive_gcp.go`
- **vSphere Tests** → Add to `hive_vsphere.go`
- **Platform-agnostic Tests** → Add to `hive.go`

### Why This Rule Exists:
1. **Maintainability**: Keeps related tests organized by platform
2. **Consistency**: Follows established Hive test architecture
3. **Efficiency**: Leverages existing platform-specific setup and utilities
4. **Code Review**: Easier to review platform-specific changes
5. **Execution**: Platform-specific files have proper skip logic and setup

### Correct vs Incorrect Approach:
```go
// ✅ CORRECT: Add to existing platform file
// File: hive_azure.go
g.It("NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:username-Medium-HIVE-XXXX-New Azure feature test [Serial]", func() {
    // Test implementation
})

// ❌ WRONG: Creating new file
// File: hive_new_feature.go (DO NOT CREATE)
g.It("Test description", func() {
    // Test implementation
})
```

## Rule 0: RFC 1123 Naming Compliance

### 🚨 CRITICAL: testCaseID Naming Convention
**ALL resource names in Kubernetes must follow RFC 1123 subdomain naming rules.**

#### MANDATORY Naming Rule:
When converting JIRA issue key to testCaseID:
- **Convert to lowercase**
- **Remove all symbols (hyphens, underscores, etc.)**
- **Use only alphanumeric characters**

#### Examples:
```go
// JIRA Key: HIVE-2883
testCaseID := "hive2883"  // ✅ CORRECT

// JIRA Key: CCO-1234
testCaseID := "cco1234"   // ✅ CORRECT

// JIRA Key: HIVE-2923
testCaseID := "HIVE-2923" // ❌ WRONG - contains uppercase and hyphens
testCaseID := "hive-2923" // ❌ WRONG - contains hyphens
```

#### Why This Rule Exists:
- **Kubernetes Validation**: RFC 1123 compliance for ClusterImageSet and other resources
- **DNS Compatibility**: Resource names must be valid DNS subdomain names
- **Consistency**: Standardized naming across all test cases

---

## Rule 1: Test Requirements Analysis and Understanding

### 🔍 MANDATORY: Test Requirements Analysis
**BEFORE generating any E2E test code, thoroughly analyze the test requirements:**

1. **JIRA Issue Analysis**:
   - Extract the core functionality being tested
   - Identify the specific component behavior under test
   - Understand the business requirement and acceptance criteria
   - Map requirements to Hive component capabilities

2. **Functional Understanding**:
   - What specific Hive functionality is being tested?
   - Which Hive resources are involved? (ClusterDeployment, MachinePool, ClusterPool, etc.)
   - What is the expected behavior vs. actual behavior?
   - What are the success criteria and failure conditions?

3. **Platform and Environment Requirements**:
   - Which cloud platform(s) are involved?
   - What are the specific configuration requirements?
   - Are there any special conditions or edge cases?
   - What are the resource requirements and constraints?

### 📋 Test Requirements Mapping Template:
```yaml
test_requirements:
  jira_key: "HIVE-XXXX"
  core_functionality: "What specific Hive feature is being tested?"
  component_resources: ["ClusterDeployment", "MachinePool", "ClusterPool"]
  platform_requirements: ["AWS", "Azure", "GCP", "vSphere"]
  success_criteria: "What defines successful test execution?"
  failure_conditions: "What conditions should cause test failure?"
```

## Rule 2: Mandatory Steps Integration

**ALWAYS analyze and apply patterns from existing test code:**

1. **Platform Detection and File Selection**:
   ```yaml
   platform_mapping:
     azure: "hive_azure.go"
     aws: "hive_aws.go"
     gcp: "hive_gcp.go"
     vsphere: "hive_vsphere.go"
     default: "hive.go"
   ```

2. **Pattern Learning Process**:
   - **Step 1**: Analyze 2-3 existing similar test functions in target file
   - **Step 2**: Extract common patterns for cluster lifecycle management
   - **Step 3**: Understand timing and wait patterns from existing code
   - **Step 4**: Apply learned patterns to new test case requirements

3. **Mandatory Pattern Analysis**:
   ```go
   // MANDATORY: Analyze existing tests for these patterns
   - "How do existing tests create ClusterDeployment?"
   - "What wait conditions do they use for cluster provisioning?"
   - "How do they handle MachinePool creation and validation?"
   - "What event checking patterns are used?"
   - "How is scaling tested in existing code?"
   - "What cleanup patterns are followed?"
   ```

4. **Pattern Application**:
   - Use similar timeout values for similar operations
   - Apply the same validation patterns for status checking
   - Implement cleanup following existing patterns
   - Structure test steps consistently with existing code

## Rule 3: Determine the Target File

Hive tests use a **platform-separated file structure**. Each platform has its dedicated test file:

| Platform    | Target File      | Description |
|------------|----------------|-------------|
| AWS        | `hive_aws.go`  | AWS-specific cluster management tests |
| Azure      | `hive_azure.go` | Azure-specific cluster management tests |
| GCP        | `hive_gcp.go`  | GCP-specific cluster management tests |
| vSphere    | `hive_vsphere.go` | vSphere-specific cluster management tests |
| Platform-agnostic | `hive.go` | Platform-independent tests |
| Utility functions | `hive_util.go` | Shared utility functions and structs |

### Actual File Structure
```
hive/
├── README.md - Documentation
├── hive.go - Platform-independent tests
├── hive_aws.go - AWS-specific tests (404KB - largest file)
├── hive_azure.go - Azure-specific tests (44KB)
├── hive_gcp.go - GCP-specific tests (71KB)
├── hive_vsphere.go - vSphere-specific tests (6KB)
├── hive_util.go - Utility functions and structs (110KB)
```

### Standard Structure for Each Test File
```go
package hive

import (
    // Platform-specific imports (e.g., AWS SDK, Azure SDK)
    g "github.com/onsi/ginkgo/v2"
    o "github.com/onsi/gomega"
    exutil "github.com/openshift/openshift-tests-private/test/extended/util"
    "github.com/openshift/openshift-tests-private/test/extended/util/architecture"
    e2e "k8s.io/kubernetes/test/e2e/framework"
)

//
// Hive test case suite for [Platform]
//

var _ = g.Describe("[sig-hive] Cluster_Operator hive should", func() {
    defer g.GinkgoRecover()
    
    var (
        oc           = exutil.NewCLI("hive", exutil.KubeConfigPath())
        ns           hiveNameSpace
        og           operatorGroup
        sub          subscription
        hc           hiveconfig
        testDataDir  string
        testOCPImage string
        // Platform-specific variables
    )
    
    g.BeforeEach(func() {
        // Skip ARM64 arch
        architecture.SkipNonAmd64SingleArch(oc)
        
        // Skip if running on non-target platform (platform-specific files only)
        exutil.SkipIfPlatformTypeNot(oc, "platform-name")
        
        // Install Hive operator if non-existent
        testDataDir = exutil.FixturePath("testdata", "cluster_operator/hive")
        _, _ = installHiveOperator(oc, &ns, &og, &sub, &hc, testDataDir)
        
        // Platform-specific setup
    })
    
    // Multiple g.It() test cases
})
```

## Rule 4: Test Case Naming Convention

### Standard format based on actual tests:
```
NonHyperShiftHOST-[Duration]-[Release]-[Connectivity]-Author:[name]-[Priority]-[JIRA-KEY]-[description] [Tags]
```

### Real Examples from Hive Tests:

**Azure Tests:**
```go
// Standard Azure test
g.It("NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:jshu-High-25447-High-28657-High-45175-[Mag] Hive API support for Azure [Serial]", func() {

// ClusterPool test
g.It("NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:jshu-Medium-33854-Hive supports Azure ClusterPool [Serial]", func() {

// Hibernation test
g.It("NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:mihuang-Medium-35297-Hive supports cluster hibernation[Serial]", func() {

// MachinePool test
g.It("NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:lwan-Medium-52415-[Azure]Hive Machinepool test for autoscale [Serial]", func() {

// Recent Azure Government test
g.It("NonHyperShiftHOST-NonPreRelease-Author:mihuang-Medium-HIVE-2544-Machinepool functionality in Azure regions without availability zone support [Serial]", func() {
```

### Common Naming Components:
- **Duration**: `Longduration` (long-running tests)
- **Release**: `NonPreRelease` (stable release tests)
- **Connectivity**: `ConnectedOnly` (requires internet connectivity)
- **Priority**: `High`, `Medium`, `Critical`, `LEVEL0`
- **Tags**: `[Serial]` (sequential execution), `[Mag]` (management tests)

## Rule 5: Platform-Specific Setup Patterns

### Common Setup Pattern:
All platform-specific files follow this basic structure:
```go
g.BeforeEach(func() {
    // 1. Skip ARM64 architecture
    architecture.SkipNonAmd64SingleArch(oc)
    
    // 2. Skip if running on wrong platform
    exutil.SkipIfPlatformTypeNot(oc, "platform-name")
    
    // 3. Install Hive operator
    testDataDir = exutil.FixturePath("testdata", "cluster_operator/hive")
    _, _ = installHiveOperator(oc, &ns, &og, &sub, &hc, testDataDir)
    
    // 4. Platform-specific setup
})
```

### AWS-Specific Setup (hive_aws.go):
```go
g.BeforeEach(func() {
    architecture.SkipNonAmd64SingleArch(oc)
    exutil.SkipIfPlatformTypeNot(oc, "aws")
    testDataDir = exutil.FixturePath("testdata", "cluster_operator/hive")
    _, _ = installHiveOperator(oc, &ns, &og, &sub, &hc, testDataDir)
    
    // AWS-specific variables
    region = getCurrentRegionFromSupportRange(oc, "aws", AWSSupportRegions)
    basedomain = getBaseDomain(oc)
    testOCPImage = getTestOCPImage(oc)
})
```

### Azure-Specific Setup (hive_azure.go):
```go
g.BeforeEach(func() {
    architecture.SkipNonAmd64SingleArch(oc)
    exutil.SkipIfPlatformTypeNot(oc, "azure")
    testDataDir = exutil.FixturePath("testdata", "cluster_operator/hive")
    _, _ = installHiveOperator(oc, &ns, &og, &sub, &hc, testDataDir)
    
    // Azure-specific variables
    region = getCurrentRegionFromSupportRange(oc, "azure", azureSupportRegions)
    basedomain = getBaseDomain(oc)
    isGovCloud = IsGovCloud(oc)
    if isGovCloud {
        cloudName = AzureGov
    } else {
        cloudName = AzurePublic
    }
})
```

### GCP-Specific Setup (hive_gcp.go):
```go
g.BeforeEach(func() {
    architecture.SkipNonAmd64SingleArch(oc)
    exutil.SkipIfPlatformTypeNot(oc, "gcp")
    testDataDir = exutil.FixturePath("testdata", "cluster_operator/hive")
    _, _ = installHiveOperator(oc, &ns, &og, &sub, &hc, testDataDir)
    
    // GCP-specific variables
    region = getCurrentRegionFromSupportRange(oc, "gcp", GCPSupportRegions)
    basedomain = getBaseDomain(oc)
    testOCPImage = getTestOCPImage(oc)
})
```

### Platform-Agnostic Setup (hive.go):
```go
g.BeforeEach(func() {
    architecture.SkipNonAmd64SingleArch(oc)
    // No platform restriction for platform-agnostic tests
    testDataDir = exutil.FixturePath("testdata", "cluster_operator/hive")
    _, _ = installHiveOperator(oc, &ns, &og, &sub, &hc, testDataDir)
    
    // Dynamic platform detection
    iaasPlatform = exutil.CheckPlatform(oc)
    testOCPImage = getTestOCPImage(oc)
})
```

## Rule 6: Hive-Specific Testing Patterns

### 🔍 MANDATORY: Pattern Analysis from Existing Tests
**ALWAYS analyze existing test patterns before creating new tests:**

1. **Cluster Lifecycle Patterns**:
   ```go
   // MANDATORY: Learn from existing cluster creation patterns
   - "How do existing tests create ClusterDeployment?"
   - "What timeout values are used for cluster provisioning?"
   - "How do existing tests wait for cluster installation completion?"
   - "What validation patterns are used for cluster status?"
   ```

2. **MachinePool Management Patterns**:
   ```go
   // MANDATORY: Learn from existing MachinePool patterns
   - "When do existing tests create MachinePools relative to cluster creation?"
   - "How do they wait for MachinePool readiness?"
   - "What validation steps are used for MachinePool functionality?"
   - "How is scaling tested in existing code?"
   ```

3. **Event Checking and Validation Patterns**:
   ```go
   // MANDATORY: Learn from existing validation patterns
   - "How do existing tests check for specific error conditions?"
   - "What event validation patterns are used?"
   - "How are timeout failures handled?"
   - "What newCheck() patterns are commonly used?"
   ```

### Key Test Data Directory:
```go
testDataDir = exutil.FixturePath("testdata", "cluster_operator/hive")
```

### Available Templates (from actual testdata):
- **ClusterDeployment**: `clusterdeployment-azure.yaml`, `clusterdeployment-aws.yaml`, `clusterdeployment-gcp.yaml`
- **ClusterPool**: `clusterpool-azure.yaml`, `clusterpool.yaml`, `clusterpool-gcp.yaml`  
- **MachinePool**: `machinepool-worker-azure.yaml`, `machinepool-infra-aws.yaml`
- **Install Config**: `azure-install-config.yaml`, `aws-install-config.yaml`, `gcp-install-config.yaml`
- **Hive Config**: `hiveconfig.yaml`, `subscription.yaml`, `operatorgroup.yaml`

### Common Test Flow Pattern:
```go
g.It("Test description", func() {
    // CRITICAL: Convert JIRA key to lowercase and remove symbols for RFC 1123 compliance
    // Example: HIVE-2883 becomes "hive2883"
    testCaseID := "hive2883"  // Convert HIVE-XXXX to lowercase without symbols
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

### Standard Utility Functions (from hive_util.go):
- `installHiveOperator()` - Install Hive operator
- `newCheck()` - Create validation checks
- `getRandomString()` - Generate random strings
- `getBaseDomain()` - Get cluster base domain
- `getCurrentRegionFromSupportRange()` - Get supported regions
- `IsGovCloud()` - Check if running in government cloud

## Rule 7: Forbidden Actions & Correct Practices

### Forbidden Actions:
- Creating new test files (use existing platform files)
- Modifying shared utility functions in `hive_util.go` without coordination
- Adding tests without proper platform detection
- Skipping architecture checks
- Missing cleanup operations
- **Generating tests without analyzing existing patterns from e2e_mandatory_steps.yaml**
- **Ignoring test requirements analysis and functional understanding**
- **Using generic patterns instead of learning from existing Hive test code**

### Correct Practices:
- **ALWAYS analyze test requirements and functional understanding first**
- **ALWAYS follow patterns from e2e_mandatory_steps.yaml**
- **ALWAYS learn from existing test code in target platform file**
- Add tests to appropriate platform-specific file
- Use existing utility functions from `hive_util.go`
- Include proper platform detection with `exutil.SkipIfPlatformTypeNot()`
- Follow established naming conventions
- Use templates from `testdata/cluster_operator/hive/`
- Include proper cleanup with `defer` statements
- Use `exutil.By()` for step organization
- **Apply learned patterns consistently with existing code**

## Rule 8: Hive Test Categories

### By Platform:
- **AWS**: Cluster lifecycle, PrivateLink, AssumeRole, IAM integration
- **Azure**: Cluster lifecycle, Government cloud, Workload Identity
- **GCP**: Cluster lifecycle, Private Service Connect, Shared VPC
- **vSphere**: On-premises cluster management
- **Platform-agnostic**: Operator health, SyncSet, ClusterImageSet

### By Functionality:
- **Cluster Lifecycle**: Provisioning, hibernation, destruction
- **ClusterPool**: Pool management, claiming, scaling
- **MachinePool**: Worker node management, autoscaling
- **Day-2 Operations**: SyncSet, cluster adoption, upgrades
- **Networking**: PrivateLink, PSC, custom networking

## Rule 9: Test Data Usage Patterns

### Template Usage:
```go
// Platform-specific cluster deployment
cluster := azureClusterDeployment{
    fake:                "false",  // Set to "true" after testing for debugging
    name:                cdName,
    namespace:           oc.Namespace(),
    baseDomain:          basedomain,
    clusterName:         cdName,
    platformType:        "azure",
    credRef:             AzureCreds,
    region:              region,
    resGroup:            AzureRESGroup,
    azureType:           cloudName,
    imageSetRef:         cdName + "-imageset",
    installConfigSecret: cdName + "-install-config",
    pullSecretRef:       PullSecret,
    template:            filepath.Join(testDataDir, "clusterdeployment-azure.yaml"),
}
```

## ⚠️ CRITICAL RULE: Fake Configuration for Debugging

### 🚨 MANDATORY FAKE CONFIGURATION RULE
**ALWAYS set `fake: "false"` during initial test generation and development.**

### 🔄 FAKE CONFIGURATION WORKFLOW:
1. **Initial Generation**: Set `fake: "false"` for real cluster deployment testing
2. **After Verification**: Change to `fake: "true"` for debugging and iteration
3. **Development Mode**: Use `fake: "true"` to reduce resource consumption

### 📋 FAKE CONFIGURATION IMPLEMENTATION:
```go
// Platform-specific cluster deployment
cluster := azureClusterDeployment{
    fake:                "false",  // ⚠️ CRITICAL: Set to "false" initially for real testing
    name:                cdName,
    namespace:           oc.Namespace(),
    baseDomain:          basedomain,
    clusterName:         cdName,
    platformType:        "azure",
    credRef:             AzureCreds,
    region:              region,
    resGroup:            AzureRESGroup,
    azureType:           cloudName,
    imageSetRef:         cdName + "-imageset",
    installConfigSecret: cdName + "-install-config",
    pullSecretRef:       PullSecret,
    template:            filepath.Join(testDataDir, "clusterdeployment-azure.yaml"),
}
```

### 🎯 FAKE CONFIGURATION BENEFITS:
- **Debugging**: Facilitate faster debugging without real cluster creation
- **Resource Efficiency**: Reduce cloud resource consumption during development
- **Iteration Speed**: Enable faster test iterations and validation
- **Cost Control**: Minimize cloud costs during test development

### ⚡ FAKE CONFIGURATION SWITCHING:
```go
// For Development/Debugging: Change to "true"
fake: "true"  // Enables fake mode for faster iteration

// For Production Testing: Keep as "false"  
fake: "false" // Enables real cluster deployment
```

### 🔧 FAKE CONFIGURATION VALIDATION:
- **Real Testing**: `fake: "false"` → Creates actual clusters for validation
- **Debug Mode**: `fake: "true"` → Skips cluster creation for faster development
- **Always Verify**: Test with `fake: "false"` before final submission

---

## Summary

Hive E2E tests follow a **platform-separated architecture** with dedicated files for each cloud provider. Each platform file includes comprehensive tests for cluster lifecycle management, while `hive_util.go` provides shared utilities. 

### 🚨 CRITICAL SUCCESS FACTORS:
1. **Test Requirements Analysis**: Thoroughly understand the functional requirements before coding
2. **Mandatory Steps Integration**: Always follow patterns from `e2e_mandatory_steps.yaml`
3. **Pattern Learning**: Learn from existing test code rather than using generic patterns
4. **Platform-Specific Implementation**: Use the correct platform file and follow established patterns
5. **Consistent Structure**: Follow established naming conventions and leverage existing templates

### 🔑 KEY PRINCIPLES:
- **Understand First**: Analyze test requirements and functional understanding
- **Learn from Existing**: Extract patterns from existing test code
- **Apply Consistently**: Use learned patterns consistently with existing code
- **Follow Standards**: Use correct platform files, naming conventions, and templates
- **Maintain Quality**: Include proper cleanup, validation, and error handling

The key is to use the correct platform file, follow established naming conventions, and leverage existing templates and utility functions for consistent, maintainable tests.