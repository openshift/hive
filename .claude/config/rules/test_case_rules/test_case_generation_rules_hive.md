# Test Case Generation Rules - Hive Component

## Repository Context

### Working Environment
- **Current Location**: `.claude/` directory (AI test generation workspace WITHIN Hive repository)
- **Analysis Target**: Hive product source code at repository root (`../`)
- **Test Generation Target**: `.claude/test_artifacts/{COMPONENT}/{JIRA_KEY}/openshift-tests-private/` (private E2E test repository)

### Key Distinction
- **Product Code**: What we ANALYZE - Hive source code in parent directory
- **Test Code**: What we GENERATE - E2E tests in openshift-tests-private
- **DeepWiki**: Alternative source for Hive architecture analysis (local code preferred)

---

## E2E vs Manual Test Classification Rules

### Manual Test Scenarios (MANDATORY)

The following scenarios MUST be classified as **Manual Test Cases**:

#### 1. Upgrade/Retrofit Scenarios
- **Keywords**: "retrofit", "legacy", "upgrade"
- **Reason**: Requires testing with legacy resources (without feature) → upgrade to new Hive (with feature)
- **Example**: Testing backward compatibility with existing clusters

#### 2. AWS Shared VPC
- **Indicator**: Uses AWS `HostedZoneRole` field
- **Reason**: DNS hosted zone is in a different AWS account
- **Setup**: Requires manual AWS shared VPC configuration

#### 3. GCP Shared VPC
- **Indicator**: Uses GCP `NetworkProjectID` field
- **Reason**: Network resources are in a different GCP project
- **Setup**: Requires manual GCP shared VPC configuration

#### 4. Azure Resource Group
- **Indicator**: Uses Azure `ResourceGroupName` field
- **Reason**: Uses existing/pre-created resource group
- **Setup**: Requires manual preparation of specific resource group

#### 5. Special Platforms
- **Platforms**: Nutanix, OpenStack, IBM Cloud
- **Reason**: Complex manual setup or limited CI support
- **Setup**: Requires platform-specific manual configuration

### E2E Test Scenarios

All other scenarios should be E2E tests unless they meet the Manual Test criteria above.

---

## Hive-Specific Test Requirements

### DNSZone Testing (MANDATORY)

**CRITICAL**: DNSZone tests MUST follow this pattern:

1. **Create Managed DNS Cluster**: 
   - Use `hiveutil` to configure `managedDomains` in `HiveConfig`
   - Create `ClusterDeployment` to trigger DNSZone creation automatically
   - **DO NOT** create DNSZone resources directly

2. **Reference Implementation**:
   - See E2E case 24088 for proper setup pattern
   - Managed DNS ensures proper zone lifecycle management

3. **Why This Matters**:
   - DNSZones are automatically created by Hive controller
   - Direct creation bypasses controller logic
   - Managed domains ensure proper cleanup

### Platform-Specific Considerations

#### AWS Platform
- **Standard Tests**: Use regular AWS credentials
- **Shared VPC Tests**: Mark as Manual (requires HostedZoneRole setup)
- **STS/OIDC**: Can be E2E if using standard test credentials

#### GCP Platform
- **Standard Tests**: Use regular GCP credentials
- **Shared VPC Tests**: Mark as Manual (requires NetworkProjectID setup)

#### Azure Platform
- **Standard Tests**: Use regular Azure credentials
- **Existing Resource Group**: Mark as Manual (requires ResourceGroupName setup)

#### Other Platforms
- **VSphere**: E2E (if CI support available)
- **BareMetal**: E2E (if CI support available)
- **Nutanix**: Manual (complex setup)
- **OpenStack**: Manual (complex setup)
- **IBM Cloud**: Manual (complex setup)

---

## Test Case Design Guidelines

### Cluster Lifecycle Testing

1. **Provision Tests**: Create ClusterDeployment, wait for installation
2. **Deprovision Tests**: Delete ClusterDeployment, verify cleanup
3. **Hibernation Tests**: Hibernate/resume cluster, verify state transitions
4. **Upgrade Tests**: Manual only (see classification rules)

### Resource Testing Patterns

1. **ClusterDeployment**: Main resource for cluster management
2. **ClusterPool**: Pre-provisioned cluster pool management
3. **DNSZone**: Only via managed domain setup (see DNSZone requirements)
4. **SyncSet**: Resource synchronization to managed clusters
5. **MachinePool**: Additional worker node groups

### Validation Requirements

1. **Resource Creation**: Verify resources exist and are in expected state
2. **Status Conditions**: Check status conditions indicate success/failure
3. **Controller Behavior**: Verify controllers act correctly
4. **Cleanup**: Ensure resources are properly deleted

---

## Hive Module Information

- **Module Name**: `github.com/openshift/hive`
- **Repository**: `https://github.com/openshift/hive`
- **Test Repository**: `https://github.com/openshift/openshift-tests-private`

---

## Quick Classification Checklist

Use this checklist when classifying test cases:

- [ ] Does it involve upgrade/retrofit/legacy? → **Manual**
- [ ] Does it require AWS Shared VPC (HostedZoneRole)? → **Manual**
- [ ] Does it require GCP Shared VPC (NetworkProjectID)? → **Manual**
- [ ] Does it require Azure Resource Group (ResourceGroupName)? → **Manual**
- [ ] Is it for Nutanix/OpenStack/IBM Cloud? → **Manual**
- [ ] Does it test DNSZone? → Ensure uses managed domain setup
- [ ] All other scenarios → **E2E**

---

## Examples

### Example 1: Standard AWS ClusterDeployment → E2E
- Creates cluster on AWS with standard credentials
- No shared VPC requirements
- Classification: **E2E**

### Example 2: AWS Shared VPC with HostedZoneRole → Manual
- Requires DNS zone in different AWS account
- Uses HostedZoneRole field
- Classification: **Manual**

### Example 3: DNSZone Management → E2E (with proper setup)
- Uses managed domain configuration
- Creates ClusterDeployment to trigger DNSZone
- Classification: **E2E**

### Example 4: Legacy Cluster Upgrade → Manual
- Tests retrofitting new feature to old cluster
- Requires upgrade procedure
- Classification: **Manual**

---

## Notes

1. **Always check keywords**: "retrofit", "legacy", "upgrade", "shared VPC", "existing resource group"
2. **DNSZone is special**: Never create directly, always via managed domain
3. **When in doubt**: If setup is complex or requires manual infrastructure, mark as Manual
4. **Platform support**: Check if platform has CI/E2E support before marking as E2E
