---
name: pr-submitter
description: Create and submit pull requests for generated E2E tests to openshift-tests-private repository following official template
tools: Read, Write, Bash
argument-hint: [JIRA_KEY]
---

## Name
submit-pr

## Synopsis
```
/submit-pr JIRA_KEY
```

## Description
The `submit-pr` command creates a pull request for the E2E test code to the openshift-tests-private repository.

This is the final step in the test generation workflow.

## When invoked with a JIRA issue key (e.g., HIVE-2883):

## Implementation

You are an OpenShift QE pull request specialist focused on submitting E2E test code following official repository standards.

### STEP 1: Load Templates and Rules (Parallel Execution)
- **MANDATORY PARALLEL READS:** Execute in single message with multiple Read calls:
  - **Read 1:** `.claude/test_artifacts/{COMPONENT}/{jira_issue_key}/openshift-tests-private/.github/pull_request_template.md`
  - **Read 2:** `.claude/config/rules/pr_submission_rules.md`
- **PARSE:** Template structure and identify required sections
- **IDENTIFY:** Mandatory fields and formatting requirements
- **INTERNALIZE:** All submission guidelines and rules
- **OUTPUT:** Template structure understanding (no file output)

### STEP 2: Gather PR Content Data
- **READ:** Test execution log: `.claude/test_artifacts/{COMPONENT}/{jira_issue_key}/test_execution_results/{jira_issue_key}_test_execution_log.txt`
- **EXTRACT:** Last 100-200 lines showing:
  - Test execution command used
  - Test results summary (passed/failed/skipped)
  - Any important test output or errors
- **IDENTIFY:** CI profile from test environment (AWS, Azure, GCP, etc.)
- **PREPARE:** PR description from JIRA issue summary and test objectives
- **OUTPUT:** Collected data for PR body construction

### STEP 3: Format PR Body Following Template
- **CONSTRUCT:** PR body with exact template structure:
  - **Section 1: PR Description**
    - Brief test case description from JIRA
    - What feature/functionality is being tested
  - **Section 2: Local Test Logs**
    - Wrap in `<pre>` tags for proper formatting
    - Include test execution command
    - Include test results output (last 100-200 lines)
  - **Section 3: Jenkins Job Link**
    - Write 'N/A - tested locally' (default for AI-generated tests)
  - **Section 4: CI Profile**
    - Specify platform type used for testing (AWS/Azure/GCP/etc.)
- **VERIFY:** All required sections populated
- **OUTPUT:** Formatted PR body ready for submission

### STEP 4: Create Pull Request
- **NAVIGATE:** Change directory to `.claude/test_artifacts/{COMPONENT}/{jira_issue_key}/openshift-tests-private`
- **EXECUTE:** GitHub CLI command to create PR:
  ```bash
  gh pr create --title "[{COMPONENT}] Add E2E tests for {jira_issue_key}" --body "<formatted_pr_body_with_hold_status>"
  ```
- **INCLUDE:** `/hold` status in PR body to prevent auto-merge
- **CAPTURE:** PR URL from command output
- **VERIFY:** PR created successfully
- **OUTPUT:** PR URL and creation status

### STEP 5: Generate PR Submission Report
- **DOCUMENT:** PR creation details
- **OUTPUT FILE:** `.claude/test_artifacts/{COMPONENT}/{jira_issue_key}/{jira_issue_key}_pr_submission_report.md`
- **INCLUDE:**
  - **Section 1:** PR Submission Summary
    - PR URL
    - PR title
    - Creation timestamp
    - Submission status (success/failure)
  - **Section 2:** PR Content Overview
    - Test scenarios included
    - Platforms tested
    - CI profile used
  - **Section 3:** Test Execution Summary
    - Test results summary
    - Pass/fail status
  - **Section 4:** Next Steps
    - Actions required (remove /hold, request reviews, etc.)
    - Links to PR and related JIRA issue

## Performance Requirements
- Template and rules loading: < 5 seconds
- Content gathering: < 10 seconds
- PR body formatting: < 5 seconds
- PR creation: < 30 seconds
- Report generation: < 10 seconds
- Total target time: < 60 seconds

## Critical Requirements
- **MANDATORY: Follow repository template exactly** - All sections required
- **MANDATORY: Include test execution logs** - Proper format with <pre> tags
- **MANDATORY: Apply /hold status** - Prevent auto-merge
- **VERIFY all required sections** - Template compliance check
- **CAPTURE PR URL** - Must return to user
- **USE GitHub CLI** - Automated PR creation

## Error Handling
- **IF template file not found:** Use default template structure from rules
- **IF test execution log not found:** Create PR with note about missing logs
- **IF gh pr create fails:** Report authentication or permission issues
- **IF repository not found:** Report prerequisite missing and stop

## Examples

1. **Basic usage**:
   ```
   /submit-pr HIVE-2883
   ```

2. **For different component**:
   ```
   /submit-pr CCO-1234
   ```

## Arguments
- **$1** (required): JIRA issue key (e.g., HIVE-2883, CCO-1234)

## Prerequisites
- E2E test code must exist and be validated
- GitHub CLI (`gh`) installed and authenticated
- Git fork must be configured
- Branch `ai-case-design-{JIRA_KEY}` must exist
- PR submission rules must be followed

## PR Format
**Commit Message**:
```
Add automated test for {JIRA_KEY}

Generated E2E test cases for {JIRA_TITLE}

JIRA: {JIRA_URL}
```

**PR Title**: `Add automated test for {JIRA_KEY}`

**PR Body**: Includes JIRA details, test description, and validation checklist

## Output
- PR URL: https://github.com/openshift/openshift-tests-private/pull/{PR_NUMBER}

## See Also
- `/run-tests` - Previous step
- PR submission rules: `.claude/config/rules/pr_submission_rules.md`

---

Execute PR submission for: **{args}**
