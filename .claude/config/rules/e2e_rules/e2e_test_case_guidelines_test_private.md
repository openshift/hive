# E2E Test Case Guidelines for Hive (Simplified)

## 🚨 MANDATORY RULES (MANDATORY - Must Follow ALL)

1. **NEVER create new test files** - Use existing platform files (hive_aws.go, hive_azure.go, etc.)
2. **ALWAYS learn from existing tests** - Analyze 3-5 existing tests before writing
3. **Use createCD() function** - Don't manually create imageSet and ClusterDeployment separately
4. **Follow RFC 1123** - Convert JIRA keys: `HIVE-2544` → `testCaseID := "hive2544"`
5. **Include proper cleanup** - Use defer statements for all resources
6. **For DNS tests** - Extract hiveutil, use enableManagedDNS(), and get DNSZone name dynamically (NEVER hardcode)

---

## 📋 Platform File Mapping

| Platform | File |
|----------|------|
| AWS | `hive_aws.go` |
| Azure | `hive_azure.go` |
| GCP | `hive_gcp.go` |
| vSphere | `hive_vsphere.go` |

---

## 🔍 Test Naming Convention

```go
g.It("NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:{AUTHOR}-Medium-{JIRA_KEY}-{Description} [Serial]", func() {
```

**Example:**
```go
g.It("NonHyperShiftHOST-NonPreRelease-Author:mihuang-Medium-HIVE-2544-Machinepool in Azure non-AZ regions [Serial]", func() {
```

---

## 🧠 Pattern Learning (MANDATORY)

### Before Writing Tests:
```bash
# Analyze existing tests in target platform file
grep -A 50 "g\.It.*NonHyperShiftHOST" hive_aws.go | head -200
```

### Extract These Patterns:
1. **Variable naming**: `testCaseID`, `poolName`, `cdName`
2. **Step organization**: `exutil.By("Step description")`
3. **Validation**: `newCheck("expect", "get", asAdmin, ...)`
4. **Cleanup**: `defer cleanupObjects(...)`

---

## 📋 Test Structure Template

### ClusterDeployment Test
```go
testCaseID := "hiveXXXX"  // RFC 1123: lowercase, no symbols
cdName := "cluster-" + testCaseID + "-" + getRandomString()[:ClusterSuffixLen]

exutil.By("Creating ClusterDeployment")
// Use createCD() or platform template

exutil.By("Waiting for cluster installation")
// Use newCheck() for validation

exutil.By("Testing functionality")
// Test-specific logic

exutil.By("Cleanup")
defer cleanupObjects(...)
```

### MachinePool Test
```go
testCaseID := "hiveXXXX"
cdName := "cluster-" + testCaseID + "-" + getRandomString()[:ClusterSuffixLen]

exutil.By("Creating ClusterDeployment")
// Create CD first

exutil.By("Creating MachinePool")
// Use machinepool template

exutil.By("Validating MachinePool")
// Wait for ready status

defer cleanupObjects(...)
```

---

## 🌐 Platform-Specific Checks

### Azure Government Cloud
```go
if !isGovCloud {
    g.Skip("Test requires Azure Government Cloud, current: " + region)
}
```

### AWS Government Cloud
```go
if !isGovCloud {
    g.Skip("Test requires AWS Government Cloud, current: " + region)
}
```

---

## 🔍 Managed DNS Setup (MANDATORY for manageDNS tests)

**MANDATORY Reference**: Test case 24088

### ✅ CORRECT Pattern
```go
hiveutilPath := extractHiveutil(oc, tmpDir)
e2e.Logf("hiveutil extracted to %v", hiveutilPath)

// Step 2: Enable managed DNS
exutil.By("Enable managed DNS using hiveutil")
enableManagedDNS(oc, hiveutilPath)


//MANDATORY: Get DNSZone name dynamically - NEVER hardcode
dnsZoneName, _, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("dnszone", "-n", oc.Namespace(), "-o=jsonpath={.items[0].metadata.name}").Outputs()
o.Expect(err).NotTo(o.HaveOccurred())
e2e.Logf("DNSZone created: %s", dnsZoneName)
```

### ❌ WRONG Pattern (Don't Do This)
```go
// ❌ Don't manually create imageSet and cluster separately
imageSet := clusterImageSet{...}
imageSet.create(oc)
cluster := clusterDeployment{...}
cluster.create(oc)

// ❌ Don't hardcode DNSZone name
dnsZoneName := cdName + "-zone"  // WRONG!
```

---

## 🚨 kubectl/oc Command Rules

### ❌ FORBIDDEN (Invalid Syntax)
```bash
# NEVER use -A with specific resource names (will ALWAYS fail)
oc get dnszone my-zone -A -o=jsonpath={...}
oc get clusterdeployment my-cd -A -o=jsonpath={...}
```

### ✅ CORRECT Syntax
```bash
# Option 1: Specific namespace + specific name
oc get dnszone my-zone -n <namespace> -o=jsonpath={...}

# Option 2: All namespaces WITHOUT specific name
oc get dnszone -A -o=jsonpath={.items[*].metadata.name}
```

---

## 🚫 Forbidden Actions

- ❌ Creating new test files
- ❌ Modifying `hive_util.go` without coordination
- ❌ Skipping platform detection
- ❌ Missing cleanup operations
- ❌ Using `-A` flag with specific resource names
- ❌ Creating inline YAML for resources
- ❌ Redundant logging (use `exutil.By()` only for key steps)

---

## 🔧 Template Strategy Decision

**Check CRD field compatibility before modifying templates:**

```bash
# Check if new field is optional
oc get crd clusterdeployments.hive.openshift.io -o yaml | grep -A 5 newField
```

**Decision Matrix:**

| Field Type | Action |
|------------|--------|
| Optional + omitempty | Merge to existing template or use JSON patch |
| Required field | Create new template |
| Backward compatible | Merge to existing template |

---

## 📝 MachinePool Non-AZ Region Testing

**For regions without availability zones (e.g., usgovtexas):**

```go
// Expect ONLY ONE MachineSet (not multiple zone-specific ones)
machineSetNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(
    "--kubeconfig="+kubeconfig, "MachineSet", "-n", "openshift-machine-api",
    "-l", "hive.openshift.io/machine-pool=worker",
    "-o=jsonpath={.items[*].metadata.name}").Output()

machineSetNameList := strings.Fields(machineSetNames)
o.Expect(len(machineSetNameList)).To(o.Equal(1), "Expected 1 MachineSet for non-AZ region")
```

---

## 🌍 Testing Region Override

```go
// Override region for specific tests
spokeRegion := "usgovtexas"     // Azure Gov
spokeRegion := "eastus"         // Azure Public
spokeRegion := "us-east-1"      // AWS
```

---

## ✅ Best Practices Summary

1. **Learn first**: Analyze existing tests before writing
2. **Follow patterns**: Variable names, validation, cleanup
3. **Use utilities**: createCD(), enableManagedDNS, newCheck()
4. **Proper syntax**: `-n <namespace>` with specific names
5. **Clean code**: Use defer, avoid redundant logs
6. **RFC 1123**: Lowercase JIRA keys, no symbols
7. **Reference tests**: 24088 (managed DNS), 25447 (MachinePool)

---

## 📚 Key Reference Test Cases

- **24088**: Managed DNS setup pattern
- **25447**: MachinePool test structure
- **32223**: ClusterDeployment setup process

Check these tests for detailed implementation examples.
