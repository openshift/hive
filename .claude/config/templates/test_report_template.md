# 🧾 {jira_issue_key} Test Report

## 1. Basic Information
| Item | Content |
|------|---------|
| **Test Project Name** | [Project Name] |
| **Test Request** | [JIRA Issue Link] |
| **Test Period** | [Start Date] - [End Date] |
| **Test Engineer** | [Tester Name] |

---

## 2. Test Conclusion
### 2.1 Test Pass/Fail Status
⬜ **Pass** - Can be released to production environment  
⬜ **Fail** - Needs to be fixed and re-tested  
⬜ **Conditional Pass** - Can be released after meeting the following conditions: [List conditions]  

### 2.2 Execution Summary
- **Execution Results**: Total [X] test cases executed, [X] passed, [X] failed, [XX]% pass rate.
- **Key Findings**:
  - ✅ Feature A working properly
  - ⚠️ Feature B has resource cleanup issues (HIVE-2579)
- **Overall Conclusion**:
  > Current version main functionality is usable, but it is recommended to fix high priority defects before entering the release phase.

### 2.3 Risk Assessment
| Risk Description | Probability | Impact | Mitigation | Owner | Status |
|------------------|-------------|--------|------------|-------|--------|
| [Risk 1] | High/Medium/Low | High/Medium/Low | [Mitigation plan] | [Owner] | Open |
| None | - | - | - | - | - |

---

## 3. Test Objectives & Scope
### 🎯 Test Objectives
Brief description of the main goals of this test, for example:
> Verify if HIVE-2579 fix is effective; verify 4.17 version PrivateLink mode installation and deletion process.

### 📍 Test Scope
- **Modules Involved**:
- **Features Covered**:
- **Out of Scope**:

### 📋 Test Artifacts & Execution Results
- **Test Case Files**:
  - **E2E Test Cases**: `{jira_issue_key}_e2e_test_case.md`
  - **Manual Test Cases**: `{jira_issue_key}_manual_test_case.md`
  - **Test Coverage Matrix**: `test_coverage_matrix.md`
- **E2E PR Links**:
  - **E2E Test Code**: [PR Link to E2E code]
  - **Test Execution Logs**: See `test_execution_results/` directory

#### Test Execution Details
| Test Case ID | Test Case Name | Hive Version | Platform | Spoke Cluster Version | Status |
|--------------|----------------|--------------|----------|----------------------|--------|
| TC001 | Create Cluster (PrivateLink Mode) | 1.2.1234 | AWS | 4.16 | ✅ Pass |
| TC002 | Delete Cluster (Resource Cleanup) | 1.2.1234 | AWS | 4.16 | ❌ Fail |
| TC003 | Verify MachinePool Status Sync | 1.2.1234 | GCP | 4.15 | ✅ Pass |
| TC004 | Install on Azure PrivateLink | 1.2.1235 | Azure | 4.15 | ✅ Pass |

#### Test Summary by Platform
| Platform | Total Cases | Executed | Passed | Failed | Pass Rate |
|----------|-------------|----------|--------|--------|-----------|
| AWS | [X] | [X] | [X] | [X] | [XX]% |
| GCP | [X] | [X] | [X] | [X] | [XX]% |
| Azure | [X] | [X] | [X] | [X] | [XX]% |
| **Total** | **[X]** | **[X]** | **[X]** | **[X]** | **[XX]%** |

---

## 4. Defects & Issues Summary

### 4.1 Product Bugs
| Issue ID | Title | Severity | Platform | Status | Notes |
|----------|-------|----------|----------|--------|-------|
| HIVE-2579 | Cluster deletion resources not fully cleaned | Major | AWS | Open | Reproduced multiple times |
| HIVE-2565 | AWS PrivateLink Cluster installation failed | Blocker | AWS | Fixed | Verification passed |
| None | No product bugs found | - | - | - | - |

### 4.2 E2E Test Bugs
| Issue ID | Title | Severity | Platform | Status | Notes |
|----------|-------|----------|----------|--------|-------|
| [E2E-001] | [E2E test case issue description] | [Severity] | [Platform] | [Status] | [Notes] |
| None | No E2E test bugs found | - | - | - | - |

### 4.3 Defect Statistics by Platform
| Platform | Bug Type | Severity | Total | Open | Fixed | Verification Passed |
|----------|----------|----------|-------|------|-------|---------------------|
| AWS | Product | Blocker | [X] | [X] | [X] | [X] |
| AWS | Product | Major | [X] | [X] | [X] | [X] |
| GCP | Product | Major | [X] | [X] | [X] | [X] |
| Azure | E2E Test | Minor | [X] | [X] | [X] | [X] |

---

## 5. Appendices
- 📂 **Logs & Screenshots**:
  - Hive controller pod logs: `/tmp/hive-controller.log`
  - Provision pod logs: `/tmp/provision.log`
- 🔗 **Related Links**:
  - [PR #2344](https://github.com/openshift/hive/pull/2344)
  - [JIRA HIVE-2579](https://issues.redhat.com/browse/HIVE-2579)

---

## Usage Instructions

### Quick Fill Guide
1. Replace all `[placeholders]` with actual content
2. Check the appropriate ⬜ options
3. Fill in specific numbers, dates, and versions
4. Add relevant links and log paths
5. Customize based on actual test scenario

### Test Environment Configuration
- **Single Environment**: Keep only one row in "Test Environments" table
- **Multiple Environments**: Add rows for each tested environment (ENV-1, ENV-2, etc.)
- **Environment ID**: Use consistent IDs (ENV-1, ENV-2) throughout the report

### Key Data Sources
- **Test Results**: Extract from `test_execution_results/*_comprehensive_test_results.md`
- **Spoke Cluster Version**: Extract from test execution log files in `test_execution_results/`
- **Test Cases**: Extract from `test_cases/` 
- **JIRA Data**: Use JIRA issue details for context and requirements

### Report Principles
- Concise and focused on key findings
- Clear status indicators for quick assessment
- Actionable conclusions and recommendations
- Easy to read and understand
