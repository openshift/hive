# Hive Test Generation System

AI-powered test generation system for OpenShift Hive component, automating the creation of comprehensive E2E and manual test cases from JIRA issues.

## Overview

This system provides slash commands to automate the complete test generation workflow:

1. **Test Case Generation** - Generate test cases from JIRA issues
2. **E2E Code Generation** - Transform test cases into executable test code
3. **Test Execution** - Run tests with auto-fix capabilities
4. **Report Generation** - Create comprehensive test reports
5. **PR Submission** - Submit test code as pull requests

## Quick Start

### Complete Workflow (Recommended)

```bash
/full-workflow HIVE-2883
```

This executes the complete workflow: test case generation → E2E code generation → test execution → report generation

### Step-by-Step Workflow

```bash
# Step 1: Generate test cases
/generate-test-case HIVE-2883

# Step 2: Generate E2E code
/generate-e2e-case HIVE-2883

# Step 3: Run tests
/run-tests HIVE-2883

# Step 4: Generate report (optional)
/generate-report HIVE-2883

# Step 5: Submit PR
/submit-pr HIVE-2883
```

## Available Commands

| Command | Description | Duration |
|---------|-------------|----------|
| `/generate-test-case` | Generate test cases from JIRA | ~90s |
| `/generate-e2e-case` | Generate E2E test code | ~120s |
| `/run-tests` | Execute E2E tests | ~180s |
| `/generate-report` | Generate comprehensive report | ~45s |
| `/submit-pr` | Create pull request | ~30s |
| `/full-workflow` | Execute complete workflow | ~3-4min |
| `/regenerate-test` | Force regenerate test case | ~90s |
| `/regenerate-e2e` | Force regenerate E2E code | ~120s |

## Command Details

### `/generate-test-case JIRA_KEY`

Generates comprehensive test cases from JIRA issues including:
- Test requirements analysis
- Test strategy and coverage matrix
- E2E and manual test cases (separated by type)

**Output:**
```
.claude/test_artifacts/{COMPONENT}/{JIRA_KEY}/
├── phases/
│   ├── test_requirements_output.md
│   └── test_strategy.md
├── test_cases/
│   ├── {JIRA_KEY}_e2e_test_case.md
│   └── {JIRA_KEY}_manual_test_case.md
└── test_coverage_matrix.md
```

### `/generate-e2e-case JIRA_KEY`

Generates executable E2E test code for all detected platforms:
- Automatic platform detection (AWS, Azure, GCP, etc.)
- Parallel code generation for multiple platforms
- Quality checks and validation

**Output:**
```
.claude/test_artifacts/{COMPONENT}/{JIRA_KEY}/openshift-tests-private/
└── test/extended/{component}/
    └── {platform}.go (updated)
```

### `/run-tests JIRA_KEY`

Executes E2E tests with intelligent features:
- Auto-fix for E2E code/config issues
- Product bug vs. E2E bug classification
- Coverage matrix updates
- Comprehensive test reports

**Output:**
```
.claude/test_artifacts/{COMPONENT}/{JIRA_KEY}/test_execution_results/
├── {JIRA_KEY}_test_execution_log.txt
└── {JIRA_KEY}_comprehensive_test_results.md
```

### `/generate-report JIRA_KEY`

Creates comprehensive test report consolidating:
- Test requirements and strategy
- Test cases (E2E and manual)
- Coverage matrix with statistics
- Execution results and bug classification

**Output:**
```
.claude/test_artifacts/{COMPONENT}/{JIRA_KEY}/test_report.md
```

### `/submit-pr JIRA_KEY`

Submits E2E test code as pull request:
- Follows official PR template
- Includes test execution logs
- Applies `/hold` status
- Generates submission report

**Output:**
- PR URL: `https://github.com/openshift/openshift-tests-private/pull/{PR_NUMBER}`

### `/full-workflow JIRA_KEY`

Executes complete workflow automatically:
- Orchestrates all agents in sequence
- Validates prerequisites
- Handles errors and recovery
- Generates workflow execution report

### `/regenerate-test JIRA_KEY`

Force regenerate test cases:
- Skips prerequisite checks
- Overwrites existing files
- Useful for fixing issues or updating requirements

### `/regenerate-e2e JIRA_KEY`

Force regenerate E2E code:
- Skips prerequisite checks
- Overwrites existing files
- Useful for fixing code issues or updating tests

## Prerequisites

### Required Tools
- **JIRA MCP** - Configured and accessible for JIRA data retrieval
- **GitHub CLI (`gh`)** - Installed and authenticated
- **OpenShift CLI (`oc`)** - For test execution with cluster access
- **Git** - For repository operations

### Environment Setup
- KUBECONFIG set to valid OpenShift cluster
- Fork of `openshift-tests-private` configured
- Component rules exist in `.claude/config/rules/test_case_rules/`

## Configuration

### Directory Structure

```
.claude/
├── README.md                    # This file
├── commands/                    # Slash command implementations
│   ├── generate-test-case.md
│   ├── generate-e2e-case.md
│   ├── run-tests.md
│   ├── generate-report.md
│   ├── submit-pr.md
│   ├── full-workflow.md
│   ├── regenerate-test.md
│   ├── regenerate-e2e.md
│   └── README.md
└── config/
    ├── rules/                   # Test generation rules
    │   ├── test_case_rules/     # Test case generation rules
    │   ├── e2e_rules/           # E2E code generation rules
    │   └── pr_submission_rules.md
    └── templates/               # Output templates
        ├── test_cases_template.yaml
        ├── test_coverage_matrix_template.md
        └── test_report_template.md
```

### Rules and Templates

- **Test Case Rules**: `.claude/config/rules/test_case_rules/`
  - `unified_test_generation_rules.md` - Common rules for all components
  - `test_case_generation_rules_{component}.md` - Component-specific rules

- **E2E Rules**: `.claude/config/rules/e2e_rules/`
  - `e2e_test_case_guidelines_test_private.md` - E2E code generation guidelines

- **Templates**: `.claude/config/templates/`
  - Define output format and structure
  - Ensure consistency across generated artifacts

## Output Artifacts

All artifacts are generated in `.claude/test_artifacts/{COMPONENT}/{JIRA_KEY}/`:

### Generated Files

1. **Test Requirements** - `phases/test_requirements_output.md`
   - Component name and JIRA summary
   - Test requirements and scope
   - Affected platforms and edge cases

2. **Test Strategy** - `phases/test_strategy.md`
   - Test coverage matrix
   - Test scenarios and validation methods

3. **Test Coverage Matrix** - `test_coverage_matrix.md`
   - Scenario-based coverage table
   - Test type classification (E2E/Manual)
   - Execution status tracking

4. **E2E Test Cases** - `test_cases/{JIRA_KEY}_e2e_test_case.md`
   - Automated executable test cases
   - Platform-specific scenarios

5. **Manual Test Cases** - `test_cases/{JIRA_KEY}_manual_test_case.md`
   - Manual test scenarios
   - Setup and validation steps

6. **Test Execution Results** - `test_execution_results/{JIRA_KEY}_comprehensive_test_results.md`
   - Test execution summary
   - Bug classification (product vs. E2E)
   - Auto-fix attempts and results

7. **Test Report** - `test_report.md`
   - Consolidated report of all artifacts
   - Risk assessment and recommendations

8. **Workflow Report** - `workflow_execution_report.md`
   - Workflow execution summary
   - Agent status and outputs

## Workflow Examples

### Example 1: New Feature Testing

```bash
# Complete workflow for new feature HIVE-2883
/full-workflow HIVE-2883
```

**Result:**
- Test cases generated with platform coverage
- E2E code created for AWS, Azure, GCP
- Tests executed with auto-fix
- Comprehensive report generated

### Example 2: Update Existing Tests

```bash
# Regenerate test cases after requirements change
/regenerate-test HIVE-2883

# Regenerate E2E code
/regenerate-e2e HIVE-2883

# Run updated tests
/run-tests HIVE-2883
```

### Example 3: Manual Step-by-Step

```bash
# Generate test cases
/generate-test-case HIVE-2923

# Review and edit test cases manually
# ... make edits ...

# Generate E2E code
/generate-e2e-case HIVE-2923

# Review generated code
# ... review code ...

# Run tests
/run-tests HIVE-2923

# Generate final report
/generate-report HIVE-2923

# Submit PR
/submit-pr HIVE-2923
```

## Best Practices

### Test Case Generation
- Review JIRA issue for completeness before generation
- Verify component-specific rules exist
- Check test coverage matrix for gaps

### E2E Code Generation
- Ensure test cases are finalized before code generation
- Review generated code for platform coverage
- Validate code compiles before execution

### Test Execution
- Verify cluster connectivity before running tests
- Monitor auto-fix attempts for patterns
- Review bug classification accuracy

### PR Submission
- Ensure tests pass before submitting
- Review PR body for completeness
- Remove `/hold` after review

## Troubleshooting

### Common Issues

**Issue: JIRA MCP not accessible**
```bash
# Verify MCP configuration
# Check JIRA credentials
# Fallback to web fetch if needed
```

**Issue: Test execution fails**
```bash
# Check cluster connectivity: oc cluster-info
# Verify KUBECONFIG is set
# Review auto-fix suggestions
```

**Issue: Code generation produces errors**
```bash
# Regenerate test cases: /regenerate-test JIRA_KEY
# Verify test case format
# Check platform detection
```

**Issue: PR creation fails**
```bash
# Verify gh authentication: gh auth status
# Check fork configuration
# Review test execution logs exist
```

### Error Recovery

1. **Workflow Failure**: Review partial execution report
2. **Agent Failure**: Check specific agent error message
3. **Prerequisite Missing**: Run prerequisite command first
4. **Regenerate Mode**: Use `/regenerate-*` commands to force updates

## Performance Optimization

### Parallel Execution
- Rules and templates loaded in parallel
- Platform code generation runs concurrently
- Independent validation checks parallelized

### Efficiency Tips
- Use `/full-workflow` for end-to-end automation
- Review artifacts before regeneration
- Clean up old artifacts periodically

## Advanced Usage

### Custom Component Rules

Create component-specific rules in `.claude/config/rules/test_case_rules/`:

```markdown
# test_case_generation_rules_{component}.md

## E2E Test Classification
- Scenario types that should be E2E
- Platform requirements
- Component-specific patterns

## Manual Test Classification
- Scenarios requiring manual setup
- Edge cases for manual validation
```

### Custom Templates

Modify templates in `.claude/config/templates/` to customize output format:
- Test case structure
- Coverage matrix format
- Report sections and content

## Support and Documentation

### Command Help
- Each command file in `commands/` contains detailed documentation
- Run command to see inline help and examples
- Check `commands/README.md` for reference

### Configuration Help
- Review rule files for guidelines
- Check template files for format requirements
- See workflow orchestrator for agent coordination

## Contributing

When adding new commands or agents:
1. Create command file in `commands/`
2. Include frontmatter with metadata
3. Document implementation steps
4. Add examples and error handling
5. Update this README

## Version

**Version:** 1.0.0  
**Last Updated:** 2025-01-21  
**Component:** Hive Test Generation System

