---
name: e2e_test_generation_openshift_private
description: Orchestrate complete E2E test generation workflow with validation, setup, code generation, and quality checks for openshift-tests-private
tools: Read, Write, Edit, Bash, Glob, Grep, mcp_deepwiki_ask_question
argument-hint: [JIRA_KEY]
---

## Name
generate-e2e-case

## Synopsis
```
/generate-e2e-case JIRA_KEY
```

## Description
The `generate-e2e-case` command generates E2E test code based on test cases and integrates it into the openshift-tests-private repository.

This is the second step in the test generation workflow.

## Implementation

You are an OpenShift QE E2E test generation orchestrator focused on coordinating the complete test generation workflow.

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

### STEP 2: Validation Phase
- **VALIDATE:** Check existing E2E test coverage and gaps
- **VERIFY:** Prerequisite test case file exists at `.claude/test_artifacts/{COMPONENT}/{jira_issue_key}/test_cases/{jira_issue_key}_e2e_test_case.md`
- **CRITICAL:** Test case file MUST exist before proceeding to code generation
- **OUTPUT:** Validation results written to phases directory

### STEP 3: Platform Detection (MANDATORY)
- **MANDATORY:** Read test case file: `.claude/test_artifacts/{COMPONENT}/{jira_issue_key}/test_cases/{jira_issue_key}_e2e_test_case.md`
- **MANDATORY:** Scan test case names for platform keywords (AWS, Azure, GCP, VSphere, etc.)
- **MANDATORY:** Extract ALL unique platforms from test case content
- **MANDATORY:** Log detected platforms: `Detected platforms: [platform1, platform2, ...]`
- **VERIFY:** Platform list is complete before code generation

### STEP 4: Parallel Code Generation for ALL Platforms (MANDATORY)
- **CRITICAL:** Execute code generation for EACH detected platform
- **CRITICAL:** Use separate tool calls in SAME message for parallel execution
- **CRITICAL:** Each call MUST include platform parameter (platform=AWS, platform=Azure, etc.)
- **CRITICAL:** DO NOT stop after one platform - ALL platforms must be processed
- **MANDATORY:** Follow ALL rules in `e2e_test_case_guidelines_test_private.md`:
  - Learn from 3-5 existing tests before writing (use grep to analyze patterns)
  - Use existing platform files (hive_aws.go, hive_azure.go, etc.)
  - Use createCD() function (don't manually create imageSet and ClusterDeployment)
  - Follow RFC 1123 naming conventions
  - Include proper cleanup with defer statements
  - For DNS tests: extract hiveutil, use enableManagedDNS(), get DNSZone name dynamically
- **PARALLEL EXECUTION FORMAT:**
  ```
  - Platform 1: AWS
  - Platform 2: Azure
  - Platform 3: GCP
  (All in same message for true parallel execution)
  ```
- **VERIFY:** ALL platforms processed before proceeding to quality check

### STEP 5: Quality Validation
- **VERIFY:** Generated code compiles successfully
- **VERIFY:** Code meets quality standards and follows guidelines in `e2e_test_case_guidelines_test_private.md`:
  - ✅ No new test files created (code added to existing platform files)
  - ✅ Test naming follows RFC 1123 and convention
  - ✅ Uses createCD() function (not manual imageSet/CD creation)
  - ✅ Includes cleanup with defer statements
  - ✅ No hardcoded values (especially DNS zone names)
  - ✅ Correct kubectl/oc command syntax (no `-A` with specific resource names)
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
- **STOP immediately** if test case file prerequisite missing
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
   /generate-e2e-case HIVE-2883
   ```

2. **For different component**:
   ```
   /generate-e2e-case CCO-1234
   ```

## Arguments
- **$1** (required): JIRA issue key (e.g., HIVE-2883, CCO-1234)

## Prerequisites
- Test case must exist (run `/generate-test-case` first)
- GitHub CLI (`gh`) installed and authenticated
- Fork of openshift-tests-private configured
- **E2E guidelines file must exist:** `.claude/config/rules/e2e_rules/e2e_test_case_guidelines_test_private.md`

## Prerequisite Check
- Checks if test case exists in `.claude/test_artifacts/{COMPONENT}/{JIRA_KEY}/test_cases/`
- If missing, prompts to run `/generate-test-case` first

## Output Location
```
.claude/test_artifacts/{COMPONENT}/{JIRA_KEY}/openshift-tests-private/
└── test/extended/{component}/
    └── {jira_key}_test.go
```

Branch: `ai-case-design-{JIRA_KEY}`

## Regenerate Mode
Use `/regenerate-e2e` to skip prerequisite checks and force regeneration.

## See Also
- `/generate-test-case` - Previous step
- `/run-tests` - Next step: Execute tests
- `/regenerate-e2e` - Force regeneration
- `.claude/config/rules/e2e_rules/e2e_test_case_guidelines_test_private.md` - **E2E code generation guidelines (MANDATORY)**

---

Execute E2E test generation for: **{args}**
