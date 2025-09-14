# Test Case: HIVE-2544
**Component:** hive  
**Summary:** [Hive/Azure/Azure Gov] Machinepool: Unable to generate machinesets in regions without availability zone support.

## Test Overview
- **Total Test Cases:** 3
- **Test Types:** Functional, Regression, Multi-Cloud
- **Estimated Time:** 90-120 minutes

## Test Cases

### Test Case HIVE-2544_001
**Name:** Azure Machinepool Creation in Non-AZ Region  
**Description:** Verify that machinepool can successfully create machinesets in Azure regions without availability zone support  
**Type:** Functional  
**Priority:** High

#### Prerequisites
- Hive cluster deployed and operational
- Azure subscription with access to regions without availability zones (e.g., usgovtexas)
- Valid Azure credentials configured in Hive
- OpenShift cluster successfully installed in target Azure region
- `oc` CLI tool available and configured

#### Test Steps  
1. **Action:** Create a ClusterDeployment in Azure region without availability zone support (usgovtexas)      
   **Expected:** ClusterDeployment successfully provisions and reaches "Provisioned" status  

2. **Action:** Create a MachinePool resource within the same Azure region without availability zones      
   **Expected:** MachinePool resource is created successfully without "zero zones returned" errors

3. **Action:** Verify MachinePool controller reconciliation status      
   **Expected:** MachinePool controller successfully reconciles without errors in logs

4. **Action:** Check that MachineSets are generated for the MachinePool      
   **Expected:** MachineSets are created successfully with appropriate Azure configuration

5. **Action:** Validate that cluster nodes are provisioned correctly      
   **Expected:** New nodes are provisioned and join the cluster successfully

### Test Case HIVE-2544_002  
**Name:** Azure Government Cloud Machinepool Support    
**Description:** Verify machinepool functionality in Azure Government cloud regions without availability zone support    
**Type:** Multi-Cloud    
**Priority:** High  
 
#### Prerequisites  
- Hive cluster with Azure Government cloud credentials configured
- Access to Azure Government cloud subscription
- Azure Government cloud region without availability zones (e.g., usgovtexas)
- Existing ClusterDeployment in Azure Government cloud

#### Test Steps  
1. **Action:** Deploy ClusterDeployment in Azure Government cloud region without availability zones      
   **Expected:** ClusterDeployment provisions successfully in Azure Government environment    

2. **Action:** Create MachinePool targeting Azure Government cloud region without AZ support      
   **Expected:** MachinePool creation succeeds without zone-related errors    

3. **Action:** Monitor machinepool controller logs for error patterns      
   **Expected:** No "zero zones returned for region" or similar Azure Government specific errors

4. **Action:** Verify MachineSets generation in Azure Government environment      
   **Expected:** MachineSets are generated with correct Azure Government cloud configurations

5. **Action:** Validate node provisioning in Azure Government cloud      
   **Expected:** Nodes are successfully provisioned and operational in Azure Government environment

### Test Case HIVE-2544_003
**Name:** Regression Test - AZ-Supported Regions Continue Working  
**Description:** Ensure that Azure regions with availability zone support continue to function correctly after the fix  
**Type:** Regression  
**Priority:** Medium

#### Prerequisites
- Hive cluster operational
- Azure subscription with access to AZ-supported regions (e.g., eastus, westus2)
- Valid Azure credentials configured
- Existing ClusterDeployment in AZ-supported Azure region

#### Test Steps
1. **Action:** Create MachinePool in Azure region with availability zone support (eastus)      
   **Expected:** MachinePool creates successfully with availability zone distribution

2. **Action:** Verify MachineSets are generated with proper availability zone distribution      
   **Expected:** MachineSets are created across multiple availability zones as expected

3. **Action:** Compare behavior with non-AZ regions to ensure no functional regression      
   **Expected:** Both AZ and non-AZ regions work correctly without conflicts

4. **Action:** Validate that existing AZ-distributed workloads remain unaffected      
   **Expected:** Existing MachinePools and MachineSets in AZ-supported regions continue operating normally

5. **Action:** Check controller logs for any unexpected warnings or errors      
   **Expected:** Controller logs show normal operation for both AZ and non-AZ region scenarios

---
*Generated from Markdown template*