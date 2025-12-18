---
name: e2e_test_generation_openshift_private
description: Orchestrate complete E2E test generation workflow with validation, setup, code generation, and quality checks for openshift-tests-private
tools: Read, Write, Edit, Bash, Glob, Grep, mcp_deepwiki_ask_question
argument-hint: [JIRA_KEY]
regenerate-mode: true
---

## Name
regenerate-e2e

## Synopsis
```
/regenerate-e2e JIRA_KEY
```

## Description
The `regenerate-e2e` command regenerates E2E test code, **skipping all prerequisite checks** and **forcing regeneration** even if E2E code already exists.

This is the regeneration variant of `/generate-e2e-case`.

## When invoked with a JIRA issue key (e.g., HIVE-2883):

## Implementation

You are an OpenShift QE E2E test generation orchestrator focused on coordinating the complete test generation workflow.

**REGENERATE MODE ENABLED** - All prerequisite checks are skipped and existing files will be overwritten.

### Code Generation Guidelines
**MANDATORY:** All E2E test code generation MUST strictly follow the guidelines in:
- `.claude/config/rules/e2e_rules/e2e_test_case_guidelines_test_private.md`

This guideline file contains:
- ✅ Critical rules (NEVER create new test files, ALWAYS use existing platform files)
- ✅ Platform file mapping (AWS→hive_aws.go, Azure→hive_azure.go, etc.)
- ✅ Test naming conventions and RFC 1123 compliance
- ✅ Pattern learning from existing tests (MANDATORY before writing)
- ✅ Correct vs. incorrect code patterns
- ✅ Managed DNS setup procedures
- ✅ kubectl/oc command syntax rules

### STEP 1: Background Repository Setup and Branch Creation
- **CHECK:** Verify if `.claude/test_artifacts/{COMPONENT}/{jira_issue_key}/openshift-tests-private` exists
- **IF exists AND contains .git:** Repository ready, proceed to branch setup
- **IF exists BUT incomplete:** Verify repository integrity or re-clone
- **IF not exists:** Execute repository setup NOW
- **BRANCH SETUP (MANDATORY):**
  - Navigate to repository: `cd .claude/test_artifacts/{COMPONENT}/{jira_issue_key}/openshift-tests-private`
  - Ensure on master branch: `git checkout master`
  - Pull latest changes: `git pull origin master`
  - Create/recreate E2E branch: `git checkout -B {jira_issue_key}-e2e`
  - Branch naming format: `{JIRA_KEY}-e2e` (e.g., `HIVE-2923-e2e`)
- **VERIFY:** Repository ready with clean working directory and on correct branch before proceeding

### STEP 2: Validation Phase (SKIPPED IN REGENERATE MODE)
- **SKIP:** All prerequisite checks
- **FORCE:** Proceed to platform detection regardless of existing files

### STEP 3: Platform Detection (MANDATORY)
- **MANDATORY:** Read test case file: `.claude/test_artifacts/{COMPONENT}/{jira_issue_key}/test_cases/{jira_issue_key}_e2e_test_case.md`
- **MANDATORY:** Scan test case names for platform keywords (AWS, Azure, GCP, VSphere, etc.)
- **MANDATORY:** Extract ALL unique platforms from test case content
- **MANDATORY:** Log detected platforms: `Detected platforms: [platform1, platform2, ...]`
- **VERIFY:** Platform list is complete before code generation

### STEP 4: Parallel Code Generation for ALL Platforms (MANDATORY)

**BEFORE CODE GENERATION - MANDATORY PREPARATION:**
1. **READ E2E GUIDELINES:** `.claude/config/rules/e2e_rules/e2e_test_case_guidelines_test_private.md`
2. **IDENTIFY TARGET FILE:** Use existing platform file, NEVER create new test file
   - AWS → `test/extended/cluster_operator/hive/hive_aws.go`
   - Azure → `test/extended/cluster_operator/hive/hive_azure.go`
   - GCP → `test/extended/cluster_operator/hive/hive_gcp.go`
   - vSphere → `test/extended/cluster_operator/hive/hive_vsphere.go`
3. **LEARN FROM EXISTING TESTS:** Read 3-5 existing tests in target file to understand patterns
4. **EXTRACT PATTERNS:** Variable naming, test structure, validation methods, cleanup patterns

**CODE GENERATION EXECUTION:**
- **CRITICAL:** Execute code generation for EACH detected platform
- **CRITICAL:** Use separate tool calls in SAME message for parallel execution
- **CRITICAL:** Each call MUST include platform parameter (platform=AWS, platform=Azure, etc.)
- **CRITICAL:** Each call MUST reference the guidelines file path
- **CRITICAL:** Each call MUST specify existing platform file to APPEND to (NOT create new file)
- **CRITICAL:** DO NOT stop after one platform - ALL platforms must be processed
- **PARALLEL EXECUTION FORMAT:**
  ```
  - Platform 1: AWS → Append to hive_aws.go
  - Platform 2: Azure → Append to hive_azure.go
  - Platform 3: GCP → Append to hive_gcp.go
  (All in same message for true parallel execution)
  ```
- **VERIFY:** ALL platforms processed before proceeding to quality check

### STEP 5: Quality Validation
- **VERIFY:** Generated code compiles successfully
- **VERIFY:** Code meets quality standards and follows guidelines in `e2e_test_case_guidelines_test_private.md`:
  - ✅ No new test files created (code added to existing platform files)
  - ✅ Test naming follows RFC 1123 and NonHyperShiftHOST convention
  - ✅ Uses createCD() function (not manual imageSet/CD creation)
  - ✅ Includes cleanup with defer statements
  - ✅ No hardcoded values (especially DNS zone names)
  - ✅ Correct kubectl/oc command syntax (no `-A` with specific resource names)
  - ✅ Uses wait.PollUntilContextTimeout (not deprecated wait.Poll)
- **IF compilation errors:** Iterate fixes until successful
- **OUTPUT:** Quality check results to phases directory

### STEP 6: Workflow Completion Report
- **SUMMARIZE:** All completed phases and their outputs
- **LIST:** Generated test files by platform
- **CONFIRM:** Test code ready for execution
- **REPORT:** Any issues or warnings encountered

## Performance Requirements
- Repository setup: < 10 seconds (if needed)
- Validation: < 10 seconds
- Platform detection: < 5 seconds
- Code generation per platform: 2 minutes (parallel execution)
- Quality check: 2 minutes
- Total target time: 5-7 minutes for complete workflow

## Critical Requirements
- **NEVER skip platform detection** - Must scan test cases
- **NEVER process only one platform** - All platforms must be generated
- **NEVER execute platforms sequentially** - Use parallel execution
- **VERIFY each step completion** before proceeding to next step
- **FORCE regeneration** - Overwrite existing files without confirmation
- **REGENERATE code** if quality checks fail until successful

## Error Handling
- **IF repository setup fails:** Report error and stop
- **IF test case file missing:** Report prerequisite missing and stop
- **IF no platforms detected:** Report error and request manual verification
- **IF code generation fails:** Report which platform failed and error details
- **IF quality check fails:** Iterate fixes automatically until success

## Examples

1. **Basic usage**:
   ```
   /regenerate-e2e HIVE-2883
   ```

2. **For different component**:
   ```
   /regenerate-e2e CCO-1234
   ```

## Arguments
- **$1** (required): JIRA issue key (e.g., HIVE-2883, CCO-1234)

## When to Use
- Fix issues in existing E2E code
- Update E2E test based on new requirements
- Regenerate after test case updates
- Force complete regeneration

## Behavior Difference
| Aspect | `/generate-e2e-case` | `/regenerate-e2e` |
|--------|---------------------|------------------|
| Prerequisite check | ✅ Yes | ❌ No (skip) |
| Overwrite existing | ❌ No (skip if exists) | ✅ Yes (force) |
| Use case | First time generation | Update/fix existing |

## Output Location
```
.claude/test_artifacts/{COMPONENT}/{JIRA_KEY}/openshift-tests-private/
└── test/extended/cluster_operator/hive/
    └── hive_{platform}.go (test appended to existing file)
```

**IMPORTANT:** Tests are APPENDED to existing platform files, NOT created as new files.

Branch: `ai-case-design-{JIRA_KEY}` (recreated if needed)

## See Also
- `/generate-e2e-case` - Normal E2E code generation
- `/regenerate-test` - Regenerate test cases

---

**REGENERATE MODE**: Skip prerequisite checks and force regeneration for: **{args}**
