---
name: test_report_generation
description: Generate comprehensive test reports from existing test artifacts, execution results, and coverage matrix
tools: Read, Write
argument-hint: [JIRA_KEY]
---

## Name
generate-report

## Synopsis
```
/generate-report JIRA_KEY
```

## Description
The `generate-report` command generates a comprehensive test report that consolidates all test artifacts including test cases, coverage matrix, and execution results.

This is an optional step that can be executed after test execution.

## When invoked with a JIRA issue key (e.g., HIVE-2923):

## Implementation

You are an OpenShift QE test reporting specialist focused on creating comprehensive test reports.

### STEP 1: Load Report Template
- **READ:** `.claude/config/templates/test_report_template.md`
- **PARSE:** Template structure and required sections
- **IDENTIFY:** Placeholder fields to populate
- **OUTPUT:** Template structure internalized (no file output)

### STEP 2: Gather All Test Artifacts (Parallel Execution)
- **MANDATORY PARALLEL READS:** Execute in single message with multiple Read calls:
  - **Read 1:** `.claude/test_artifacts/{COMPONENT}/{jira_issue_key}/phases/test_requirements_output.md` (or .yaml)
  - **Read 2:** `.claude/test_artifacts/{COMPONENT}/{jira_issue_key}/test_coverage_matrix.md`
  - **Read 3:** `.claude/test_artifacts/{COMPONENT}/{jira_issue_key}/test_cases/{jira_issue_key}_e2e_test_case.md` (if exists)
  - **Read 4:** `.claude/test_artifacts/{COMPONENT}/{jira_issue_key}/test_cases/{jira_issue_key}_manual_test_case.md` (if exists)
  - **Read 5:** `.claude/test_artifacts/{COMPONENT}/{jira_issue_key}/test_execution_results/comprehensive_test_results.md` (if exists)
- **HANDLE:** Missing files gracefully (mark as "Not Available" in report)
- **OUTPUT:** Collected artifact data ready for report population

### STEP 3: Generate Comprehensive Report
- **POPULATE:** Template with all collected data
- **INCLUDE SECTIONS:**
  - **Section 1: Test Overview**
    - JIRA issue summary
    - Test generation date
    - Artifacts overview (list all generated files)
  - **Section 2: Test Requirements**
    - From test_requirements_output.md
    - Root cause analysis
    - Test scope and objectives
  - **Section 3: Test Coverage Matrix**
    - From test_coverage_matrix.md
    - Coverage statistics
    - Test scenarios breakdown
  - **Section 4: E2E Test Cases**
    - From e2e_test_case.md (if exists)
    - Automated test scenarios
    - Platform coverage
  - **Section 5: Manual Test Cases**
    - From manual_test_case.md (if exists)
    - Manual test scenarios
    - Platform-specific tests
  - **Section 6: Execution Results**
    - From comprehensive_test_results.md (if exists)
    - Test execution summary
    - Pass/fail statistics
  - **Section 7: Product Bugs Identified**
    - Extract from execution results
    - Bug severity and impact
    - Recommended actions
  - **Section 8: E2E Test Issues Identified**
    - Extract from execution results
    - Test code/config issues
    - Fixes applied
  - **Section 9: Risk Assessment**
    - Based on coverage and execution results
    - Identified gaps or risks
    - Mitigation recommendations
  - **Section 10: Defect Statistics**
    - Total bugs found (product vs. E2E)
    - Pass rate and coverage metrics
    - Quality score
- **OUTPUT FILE:** `.claude/test_artifacts/{COMPONENT}/{jira_issue_key}/test_report.md`

### STEP 4: Report Validation
- **VERIFY:** All required sections populated
- **VERIFY:** Report follows template structure
- **VERIFY:** File size within limits (< 20KB)
- **VERIFY:** All artifact references correct
- **OUTPUT:** Validation status and report location

## Performance Requirements
- Template loading: < 2 seconds
- Artifact gathering (parallel): < 10 seconds
- Report generation: < 30 seconds
- Validation: < 3 seconds
- Total target time: < 45 seconds

## Critical Requirements
- **FOLLOW template structure exactly** - All sections in order
- **INCLUDE all available artifacts** - Don't skip any found files
- **SEPARATE product bugs from E2E bugs** - Clear classification
- **PROVIDE actionable statistics** - Must be useful for decision-making
- **HANDLE missing files gracefully** - Mark as "Not Available" rather than failing
- **MAXIMUM report size: 20KB** - Keep concise and focused

## Error Handling
- **IF template not found:** Use default structure from memory
- **IF no artifacts found:** Create minimal report with available data
- **IF file read fails:** Mark section as "Not Available" and continue
- **IF report exceeds size limit:** Summarize sections to reduce size

## Examples

1. **Basic usage**:
   ```
   /generate-report HIVE-2883
   ```

2. **For different component**:
   ```
   /generate-report CCO-1234
   ```

## Arguments
- **$1** (required): JIRA issue key (e.g., HIVE-2883, CCO-1234)

## Prerequisites
This command requires existing test artifacts from previous workflow steps:
- test_requirements_output.md (from test_case_generation)
- test_coverage_matrix.md (from test_case_generation)
- E2E/Manual test cases (from test_case_generation)
- comprehensive_test_results.md (from test-executor, optional)

## Output Structure
```
.claude/test_artifacts/{COMPONENT}/{JIRA_KEY}/
└── test_report.md
```

## Report Contents
The generated report includes:
- JIRA issue summary
- Test requirements
- Test strategy
- Test cases (E2E and Manual)
- Test coverage matrix
- Test execution results (if available)

## See Also
- `/run-tests` - Previous step
- Report template: `.claude/config/templates/test_report_template.md`

---

Execute test report generation for: **{args}**
