# Test Case: HIVE-2923
**Component:** hive  
**Summary:** Scrub AWS RequestID from DNSZone "DNSError" status condition message (thrashing)

## Test Overview
- **Total Test Cases:** 3
- **Test Types:** Functional, Stability, Validation
- **Estimated Time:** 45 minutes

## Test Cases

### Test Case HIVE-2923_001
**Name:** DNSZone AWS RequestID Scrubbing Validation  
**Description:** Validate that AWS RequestID (UUID) is properly scrubbed from DNSZone error messages  
**Type:** Functional  
**Priority:** High

#### Prerequisites
- Hive cluster with DNSZone controller running
- AWS environment configured for DNS zone management
- Ability to configure invalid AWS credentials
- Access to DNSZone custom resources

#### Test Steps  
1. **Action:** Create DNSZone resource with manageDNS enabled and invalid AWS credentials      
   **Expected:** DNSZone is created successfully  

2. **Action:** Monitor DNSZone status conditions for DNSError events      
   **Expected:** DNSError status condition appears with authentication failure message  

3. **Action:** Inspect DNSError message content for RequestID patterns (UUID format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx)      
   **Expected:** No RequestID/UUID patterns present in error message text  

4. **Action:** Verify error message still contains meaningful authentication error information      
   **Expected:** Error message retains useful diagnostic information without RequestID  

### Test Case HIVE-2923_002  
**Name:** DNSZone Controller Stability Under AWS Errors    
**Description:** Verify DNSZone controller does not thrash when encountering repeated AWS authentication errors    
**Type:** Stability    
**Priority:** High  
 
#### Prerequisites  
- Hive cluster with DNSZone controller running
- AWS environment with invalid credentials configured
- Monitoring access to controller logs
- DNSZone resource with manageDNS enabled  

#### Test Steps  
1. **Action:** Create DNSZone with invalid AWS credentials and monitor controller behavior      
   **Expected:** Controller processes DNSZone and encounters authentication error    

2. **Action:** Monitor DNSZone status updates over 5-minute period, counting status condition changes      
   **Expected:** Status conditions stabilize without excessive updates (no thrashing behavior)    

3. **Action:** Check controller logs for excessive requeue activity or error loops      
   **Expected:** Controller follows normal backoff patterns, no immediate requeuing due to RequestID changes    

### Test Case HIVE-2923_003  
**Name:** Error Message Content Preservation    
**Description:** Validate that error scrubbing preserves useful diagnostic information while removing RequestID    
**Type:** Validation    
**Priority:** Medium  
 
#### Prerequisites  
- Hive cluster with DNSZone controller
- AWS environment for testing different error scenarios
- Access to DNSZone status conditions
- Invalid AWS credentials for multiple error types  

#### Test Steps  
1. **Action:** Generate various AWS authentication errors (AccessDenied, InvalidCredentials, etc.)      
   **Expected:** Different AWS error types are triggered and captured in DNSError conditions    

2. **Action:** Examine error messages for essential diagnostic information (service, operation, cause)      
   **Expected:** Error messages contain actionable diagnostic information for troubleshooting    

3. **Action:** Verify no RequestID or UUID patterns remain in any error message variants      
   **Expected:** All error messages are clean of RequestID patterns while maintaining diagnostic value    

4. **Action:** Compare error message usefulness before and after RequestID scrubbing      
   **Expected:** Error messages remain helpful for debugging AWS authentication issues    

---
*Generated from Markdown template*