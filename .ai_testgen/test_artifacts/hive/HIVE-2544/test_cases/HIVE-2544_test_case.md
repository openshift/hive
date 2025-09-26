# Test Case: HIVE-2544
**Component:** hive
**Summary:** [Hive/Azure/Azure Gov] Machinepool: Unable to generate machinesets in regions without availability zone support.

## Test Overview
- **Total Test Cases:** 1
- **Test Types:** Functional
- **Estimated Time:** 30 minutes

## Test Cases

### Test Case HIVE-2544_001
**Name:** Azure Machinepool Creation in Non-AZ Regions
**Description:** Validate that Hive can successfully create machinepools and generate machinesets in Azure regions that don't support availability zones
**Type:** Functional
**Priority:** Major

#### Prerequisites
- Hive cluster deployed and operational in Azure non-AZ region (e.g., usgovtexas)
- Azure or Azure Government cloud credentials configured
- Target OpenShift cluster successfully installed in non-AZ region
- ClusterDeployment resource exists and is in "Installed" state

#### Test Steps
1. **Action:** Create a MachinePool resource targeting the non-AZ Azure region
   **Expected:** MachinePool resource is created successfully without errors

2. **Action:** Monitor MachinePool reconciliation and wait for status update
   **Expected:** MachinePool controller processes the resource without "zero zones returned" errors

3. **Action:** Verify MachineSet generation by checking created MachineSets
   **Expected:** MachineSets are generated successfully with proper Azure configuration and no zone specifications

4. **Action:** Check MachinePool status and conditions for any error messages
   **Expected:** MachinePool status shows healthy condition with no zone-related error messages

5. **Action:** Validate that cluster nodes are provisioned from the MachinePool
   **Expected:** New nodes are successfully added to the cluster and reach Ready state

---
*Generated from Markdown template*