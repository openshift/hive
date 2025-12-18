---
name: test_case_generation
description: Generate comprehensive test cases for JIRA issues in OpenShift QE, including E2E and manual test cases with coverage matrix.
tools: Read, Edit, Bash, Grep, Glob, gh, jira-mcp-snowflake MCP, DeepWiki-MCP
argument-hint: [JIRA_KEY]
regenerate-mode: true
---

## Name
regenerate-test

## Synopsis
```
/regenerate-test JIRA_KEY
```

## Description
The `regenerate-test` command regenerates test cases from a JIRA issue, **skipping all prerequisite checks** and **forcing regeneration** even if test cases already exist.

This is the regeneration variant of `/generate-test-case`.

## When invoked with a JIRA issue key:
1. Gather JIRA data, extract component, then search related PRs (sequential execution)
2. Analyze requirements and generate test requirements document
3. Execute thinking framework and create test strategy with coverage matrix
4. Generate test case files

## Implementation

You are an OpenShift QE specialist focusing on test requirements analysis and test case generation.

**REGENERATE MODE ENABLED** - All prerequisite checks are skipped and existing files will be overwritten.

### STEP 1: Sequential Resource Gathering (JIRA then PR Search)
- **MANDATORY:** TOOL CALL 1 (JIRA): Use jira-mcp-snowflake MCP to get JIRA issue data for {jira_issue_key}, If MCP fails, use WebFetch for JIRA data
- **MANDATORY:** Get component name from JIRA data (from components field)
- **MANDATORY:** TOOL CALL 2 (PR): Use gh CLI to search PRs: `gh pr list --search "{jira_issue_key}" --repo openshift/{component} --state all --json url,title,number`
- **NOTE:** Step 2 depends on component from Step 1, so these must execute sequentially
- **FALLBACK:**
- **ANALYZE (if PRs found):** Use gh CLI for detailed PR analysis: `gh pr view {PR_URL}` and `gh pr diff {PR_URL}` to examine commits and file modifications
- **OUTPUT:** JIRA data + PR links + detailed PR analysis (if available); proceed to analysis even if PR search fails

### STEP 2: Generate Analysis Output
- **WAIT:** Ensure Step 1 (sequential resource gathering) completes before proceeding
- **COMBINE:** JIRA issue data + all PR change details (if any)
- **ANALYZE:** root cause, test_requirements, technical scope, affected platforms, test scenarios
- **OUTPUT:** Generate `test_requirements_output.md` to `.claude/test_artifacts/{component}/{jira_issue_key}/phases/test_requirements_output.md`
- **CRITICAL:** File MUST contain ONLY the fields defined in Output Files section (component_name, card_summary, test_requirements, affected_platforms, test_scenarios, edge_cases)
- **FORBIDDEN:** Adding any sections, headers, or content NOT explicitly listed in the Content field definition

### STEP 3: Load Rules & Execute Thinking Framework
- **MANDATORY:** Load all test generation rules from `.claude/config/rules/test_case_rules/`
- **MANDATORY:** Execute thinking framework internally following 4 phases (DO NOT output thinking process to user - execute internally only):
  - Phase 1: Ecosystem Contextualization
  - Phase 2: User Reality Immersion
  - Phase 3: Systemic Integration Analysis (includes architecture understanding)
  - Phase 4: End-to-End Value Delivery Validation
- **OUTPUT:** Generate `test_strategy.md` to `.claude/test_artifacts/{component}/{jira_issue_key}/phases/test_strategy.md`
- **CRITICAL:** test_strategy.md MUST contain ONLY: test_coverage_matrix, test_scenarios, validation_methods
- **FORBIDDEN:** Adding any sections NOT in Content field (no Executive Summary, Thinking Framework Application, etc.)
- **OUTPUT:** Generate `test_coverage_matrix.md` to `.claude/test_artifacts/{component}/{jira_issue_key}/test_coverage_matrix.md`
- **MANDATORY:** MUST follow EXACT template structure from `.claude/config/templates/test_coverage_matrix_template.md`
- **FORBIDDEN:** Adding any sections, headers, or columns NOT in the template
- **MANDATORY:** Use ONLY the table format defined in template (Scenario headers + table rows)
- **VERIFICATION:** Before writing `test_coverage_matrix.md`, verify each row's Test Type column against classification rules

### STEP 4: Generate Final Test Cases
- **MANDATORY:** Combine `test_requirements_output.md` and `test_strategy.md`
- **MANDATORY:** Load template file `.claude/config/templates/test_cases_template.md`
- **MANDATORY:** Check if component-specific classification rules exist AND have content:
  - Replace `{component}` with actual component name (e.g., "hive" → `test_case_generation_rules_hive.md`)
  - Check file exists: `test -f .claude/config/rules/test_case_rules/test_case_generation_rules_{component}.md`
  - Check file has content: `test -s .claude/config/rules/test_case_rules/test_case_generation_rules_{component}.md` (file size > 0)
  - **DECISION BASED ON RESULT:**
    - **If file EXISTS AND has content (file size > 0):** Separate test cases by type - create TWO files:
      - **FILE 1:** `{jira_issue_key}_e2e_test_case.md` - Contains ONLY E2E test cases
      - **FILE 2:** `{jira_issue_key}_manual_test_case.md` - Contains ONLY Manual test cases
    - **If file NOT FOUND OR empty (file size = 0):** Create ONE file:
      - **FILE:** `{jira_issue_key}_test_cases.md` - Contains ALL test cases (no E2E/Manual separation)
- **OUTPUT:** Generate files to `.claude/test_artifacts/{jira_issue_key}/test_cases/`
- **NOTE:** If all test cases are one type (when rules exist), only generate the corresponding file

## Performance Requirements
- Execute independent operations in parallel where possible (rules loading, template reading)
- JIRA fetch and PR search must be sequential (PR search depends on component from JIRA)
- Target completion: 60-90 seconds
- Use concise outputs: `✅ Step X completed: <brief result>`

## Rules and Guidelines
- All test generation rules: `.claude/config/rules/test_case_rules/unified_test_generation_rules.md`
- Component-specific rules: `.claude/config/rules/test_case_rules/test_case_generation_rules_{component}.md`
  - Contains classification rules (E2E vs Manual), platform requirements, and component-specific patterns
- All rules are loaded and applied during execution - refer to rule files for complete details

## Output Files

**CRITICAL PATH RULE**: All output files MUST be created in `.claude/test_artifacts/` directory

**Output Location**: `.claude/test_artifacts/{component}/{jira_issue_key}/`

**Path Format**: Always use relative paths from project root: `.claude/test_artifacts/{component}/{jira_issue_key}/...`

### Generated Files

1. **test_requirements_output.md**
   - Path: `.claude/test_artifacts/{component}/{jira_issue_key}/phases/test_requirements_output.md`
   - Format: Markdown
   - Description: Test requirements analysis
   - **MANDATORY Content ONLY:** component_name, card_summary, test_requirements, affected_platforms, test_scenarios, edge_cases
   - **FORBIDDEN:** Any additional sections (Root Cause Analysis, Technical Scope, Integration Points, etc.)

2. **test_strategy.md**
   - Path: `.claude/test_artifacts/{component}/{jira_issue_key}/phases/test_strategy.md`
   - Format: Markdown
   - Description: Test strategy and coverage
   - **MANDATORY Content ONLY:** test_coverage_matrix, test_scenarios, validation_methods
   - **FORBIDDEN:** Any additional sections (Executive Summary, Philosophy, Thinking Framework, Risk Assessment, etc.)

3. **test_coverage_matrix.md**
   - Path: `.claude/test_artifacts/{component}/{jira_issue_key}/test_coverage_matrix.md`
   - Format: Markdown
   - Description: Test coverage matrix
   - Template: `.claude/config/templates/test_coverage_matrix_template.md`
   - **CRITICAL:** MUST use EXACT template structure - ONLY scenario headers and table rows
   - **FORBIDDEN:** Adding summary sections, statistics, or any content NOT in template

4. **{jira_issue_key}_e2e_test_case.md**
   - Path: `.claude/test_artifacts/{component}/{jira_issue_key}/test_cases/{jira_issue_key}_e2e_test_case.md`
   - Format: Markdown
   - Description: E2E test cases only (automated executable test cases)
   - Template: `.claude/config/templates/test_cases_template.yaml`
   - **CRITICAL:** MUST follow EXACT template structure and variable placeholders
   - **FORBIDDEN:** Adding any sections NOT defined in template

5. **{jira_issue_key}_manual_test_case.md**
   - Path: `.claude/test_artifacts/{component}/{jira_issue_key}/test_cases/{jira_issue_key}_manual_test_case.md`
   - Format: Markdown
   - Description: Manual test cases only (require manual setup or validation)
   - Template: `.claude/config/templates/test_cases_template.yaml`
   - **CRITICAL:** MUST follow EXACT template structure and variable placeholders
   - **FORBIDDEN:** Adding any sections NOT defined in template

## Examples

1. **Basic usage**:
   ```
   /regenerate-test HIVE-2883
   ```

2. **For different component**:
   ```
   /regenerate-test CCO-1234
   ```

## Arguments
- **$1** (required): JIRA issue key (e.g., HIVE-2883, CCO-1234)

## When to Use
- Fix issues in existing test case
- Update test case based on new requirements
- Regenerate after JIRA issue updates
- Force complete regeneration

## Behavior Difference
| Aspect | `/generate-test-case` | `/regenerate-test` |
|--------|----------------------|-------------------|
| Prerequisite check | ✅ Yes | ❌ No (skip) |
| Overwrite existing | ❌ No (skip if exists) | ✅ Yes (force) |
| Use case | First time generation | Update/fix existing |

## See Also
- `/generate-test-case` - Normal test case generation
- `/regenerate-e2e` - Regenerate E2E code

---

**REGENERATE MODE**: Skip prerequisite checks and force regeneration for: **{args}**
