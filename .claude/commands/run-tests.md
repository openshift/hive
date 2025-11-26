---
name: test-executor
description: Execute E2E tests in openshift-tests-private, analyze results, perform auto-fixes if needed, and update coverage matrix
tools: Read, Write, Edit, Bash, mcp_deepwiki_ask_question
argument-hint: [JIRA_KEY]
---

## Name
run-tests

## Synopsis
```
/run-tests JIRA_KEY
```

## Description
The `run-tests` command executes E2E tests for a JIRA issue and generates comprehensive test execution reports.

This is the third step in the test generation workflow.

## When invoked with a JIRA issue key (e.g., HIVE-2883):

## Implementation

You are an OpenShift QE test execution specialist focused on comprehensive E2E test execution with intelligent failure analysis and auto-fix.

### STEP 1: Environment Validation (Parallel Checks)
- **PARALLEL EXECUTION:** Execute multiple validation commands in single message:
  - **Check 1:** Load and verify KUBECONFIG environment variable
  - **Check 2:** Verify cluster availability: `oc cluster-info`
  - **Check 3:** Verify oc version: `oc version`
  - **Check 4:** Verify cluster permissions: `oc whoami`
- **VERIFY:** Required tools available (kubectl, oc, extended-platform-tests)
- **OUTPUT:** Environment validation status with cluster details

### STEP 2: Test Environment Preparation
- **NAVIGATE:** Change directory to `.claude/test_artifacts/{COMPONENT}/{JIRA_KEY}/openshift-tests-private`
- **BUILD:** Execute `make all` to compile test binaries
- **CREATE:** Output directory: `mkdir -p ../test_execution_results`
- **VERIFY:** Build completed successfully and binaries exist
- **OUTPUT:** Build status and binary location

### STEP 3: E2E Test Execution
- **EXECUTE:** Test command (sequential execution required):
  ```bash
  ./bin/extended-platform-tests run all --dry-run | grep "{JIRA_KEY}" | ./bin/extended-platform-tests run --timeout 100m -f - --output-file ../test_execution_results/{JIRA_KEY}_test_execution_log.txt
  ```
- **WAIT:** Allow test execution to complete (timeout: 100 minutes)
- **CAPTURE:** Real exit code using `echo $?`
- **VERIFY:** Log file exists: `../test_execution_results/{JIRA_KEY}_test_execution_log.txt`
- **READ:** Test execution log to extract results
- **OUTPUT:** Test execution status and log file location

### STEP 4: Test Result Analysis + Auto-Fix (Conditional)
- **MANDATORY:** Read and analyze test execution log completely
- **EXTRACT:** All error messages, stack traces, failure points, assertion failures
- **CLASSIFY:** Errors hierarchically:
  - **E2E code issues:** Syntax errors, undefined functions, incorrect selectors
  - **E2E config issues:** Wrong parameters, missing resources, timeout issues
  - **Product bugs:** Actual product defects requiring bug reports
- **IF E2E code/config issues detected:**
  - **USE:** DeepWiki MCP to analyze correct command syntax and patterns
  - **APPLY:** Code/config corrections to fix issues
  - **RE-EXECUTE:** Tests with fixes applied (Step 3 again)
  - **DOCUMENT:** Auto-fix attempts and results
  - **MAXIMUM:** 2 auto-fix iterations
- **IF product bugs detected:**
  - **DOCUMENT:** Bug details for reporting
  - **MARK:** Tests as failed due to product defect
- **OUTPUT:** Analysis results with classifications and fix attempts

### STEP 5: Update Test Coverage Matrix
- **READ:** `../test_coverage_matrix.md`
- **PARSE:** Execution log to extract results per Test Case ID
- **UPDATE:** Status column for each test case:
  - ✅ **Passed** - Test executed successfully
  - ❌ **Failed** - Test executed but failed (product bug)
  - ⚠️ **Skipped** - Test not executed
  - 🔄 **Retried** - Failed initially, passed after auto-fix
  - 🐛 **Product Bug** - Failed due to product defect
- **CALCULATE:** Coverage metrics:
  - Total test scenarios
  - Executed scenarios
  - Skipped scenarios
  - Pass rate percentage
  - Execution coverage percentage
- **WRITE:** Updated matrix with coverage summary section
- **OUTPUT FILE:** `../test_coverage_matrix.md` (updated)

### STEP 6: Generate Comprehensive Test Report
- **CREATE:** Detailed test execution report
- **OUTPUT FILE:** `../test_execution_results/{JIRA_KEY}_comprehensive_test_results.md`
- **INCLUDE:**
  - **Section 1:** Test Execution Summary (total, passed, failed, skipped)
  - **Section 2:** Detailed Results by Test Case
  - **Section 3:** Auto-Fix Attempts and Results (if any)
  - **Section 4:** Product Bugs Identified (if any)
  - **Section 5:** E2E Test Issues Identified (if any)
  - **Section 6:** Coverage Matrix Update Summary
  - **Section 7:** Recommendations for Next Steps

## Performance Requirements
- Environment validation: < 10 seconds
- Test environment preparation: < 90 seconds
- E2E test execution: 10-90 minutes (depends on test complexity)
- Result analysis: < 5 minutes
- Auto-fix iteration: < 15 minutes (if needed)
- Coverage matrix update: < 2 minutes
- Report generation: < 3 minutes
- Total target time: 20-120 minutes (depends on test execution time)

## Critical Requirements
- **MANDATORY: Execute actual tests** - No simulated execution
- **MANDATORY: Capture real exit codes** - Must verify actual test results
- **MANDATORY: Auto-fix workflow** - Must attempt fixes for E2E issues
- **FORBIDDEN: Skip error analysis** - Must analyze all failures
- **MAXIMUM 2 auto-fix iterations** - Prevent infinite loops
- **CLASSIFY errors correctly** - E2E issues vs. product bugs
- **UPDATE coverage matrix** - Must reflect actual execution results

## Error Handling
- **IF environment validation fails:** Report cluster connectivity issues and stop
- **IF build fails:** Report compilation errors and stop
- **IF test execution fails to start:** Check binary and permissions
- **IF auto-fix fails after 2 iterations:** Document and proceed to reporting
- **IF coverage matrix not found:** Create new matrix from test results

## Examples

1. **Basic usage**:
   ```
   /run-tests HIVE-2883
   ```

2. **For different component**:
   ```
   /run-tests CCO-1234
   ```

## Arguments
- **$1** (required): JIRA issue key (e.g., HIVE-2883, CCO-1234)

## Prerequisites
- E2E test code must exist (run `/generate-e2e-case` first)
- Test environment must be configured
- OpenShift cluster kubeconfig accessible

## Output Structure
```
.claude/test_artifacts/{COMPONENT}/{JIRA_KEY}/phases/
└── comprehensive_test_results.md
```

## Test Execution Details
- Tests run against OpenShift cluster
- Results include:
  - Test pass/fail status
  - Execution logs
  - Error diagnostics (if failed)
  - Performance metrics

## See Also
- `/generate-e2e-case` - Previous step
- `/generate-report` - Generate comprehensive report
- `/submit-pr` - Next step: Submit PR

---

Execute test execution for: **{args}**
