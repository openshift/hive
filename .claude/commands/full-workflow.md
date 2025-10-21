---
name: workflow_orchestrator
description: Automatically orchestrate multi-agent workflows based on user intent
tools: Read, Write, Edit, Bash, Grep, Glob, gh, jira-mcp-snowflake MCP, DeepWiki-MCP
argument-hint: [JIRA_KEY]
---

## Name
full-workflow

## Synopsis
```
/full-workflow JIRA_KEY
```

## Description
The `full-workflow` command executes the complete test generation workflow, automating all steps from test case generation to test execution.

This command combines three agents in sequence:
1. Test case generation
2. E2E test code generation
3. Test execution and reporting

## When invoked with a JIRA issue key (e.g., HIVE-2883):

## Implementation

You are an OpenShift QE Workflow Manager focused on multi-agent workflow orchestration and dependency management.

### STEP 1: Parse User Request
- **PARSE:** Extract JIRA issue key from user request using regex pattern `(HIVE|CCO|SPLAT)-\d+`
- **PARSE:** Detect REGENERATE mode by checking for keywords: 're-create', 'recreate', 're-generate', 'regenerate'
- **PARSE:** Convert user request to lowercase for keyword matching

### STEP 2: Identify Workflow
- **MATCH:** Iterate through all workflows and check trigger_keywords against user request
- **MATCH:** Select the workflow with the most specific keyword match
- **MATCH:** If no workflow matches, default to single agent execution based on context
- **LOG:** Output identified workflow name and description

### STEP 3: Prerequisite Validation
- **VALIDATE:** For each agent in the workflow sequence:
  - IF REGENERATE mode AND skip_on_regenerate=true: SKIP prerequisite check
  - ELSE IF prerequisite exists: Check if prerequisite_check file/directory exists
  - IF prerequisite missing AND NOT regenerate mode: STOP and report missing prerequisite
  - LOG: Prerequisite validation results for each agent

### STEP 4: Execute Workflow
- **EXECUTE:** For each agent in workflow.agents sequence:
  - **STEP 4.1:** Read agent markdown config from agent.config path
  - **STEP 4.2:** Log agent execution start: '✅ Executing agent: {agent.name}'
  - **STEP 4.3:** Execute ALL steps defined in the agent's markdown config
  - **STEP 4.4:** Verify agent completion by checking output files
  - **STEP 4.5:** Log agent completion: '✅ Agent {agent.name} completed'
  - IF agent fails: STOP workflow and proceed to error handling

### STEP 5: Workflow Completion and Reporting
- **REPORT:** Generate workflow execution summary
- **REPORT:** List all executed agents and their status
- **REPORT:** List all generated output files and their locations
- **REPORT:** Calculate total execution time
- **LOG:** '✅ Workflow {workflow.name} completed successfully'

### STEP 6: Error Handling
- **ERROR:** If any agent fails during execution:
  - LOG: Error details including agent name, step number, and error message
  - STOP: Halt workflow execution immediately
  - REPORT: Generate partial workflow execution report
  - SUGGEST: Provide recovery suggestions based on error type

## Workflow Definitions

### Complete Test Generation and Execution
**Trigger Keywords:**
- "generate test cases and run"
- "complete flow for"
- "full test generation"
- "create and run tests"

**Agents:**
1. **test_case_generation** - Generate test cases - E2E or Manual or both
2. **e2e_test_generation_openshift_private** - Generate E2E test code
3. **test-executor** - Execute E2E tests
4. **test_report_generation** - Generate comprehensive test report

## Performance Requirements
- Parse request: 1 second
- Identify workflow: 1 second
- Validate prerequisites: 2-5 seconds
- Execute agents: Depends on agents
- Generate report: 2 seconds
- Total target time: 3-4 minutes for complete workflow

## Critical Requirements
- **MANDATORY: Execute actual tests** - No simulated execution
- **MANDATORY: Validate prerequisites** - Check dependencies before execution
- **MANDATORY: Handle REGENERATE mode** - Skip checks when appropriate
- **FORBIDDEN: Skip error analysis** - Must analyze all failures
- **STOP on agent failure** - Halt workflow immediately on errors

## Error Handling
- **IF workflow identification fails:** Default to single agent execution
- **IF prerequisite validation fails:** Report missing dependencies and stop
- **IF agent execution fails:** Stop workflow and provide recovery suggestions
- **IF regenerate mode detected:** Skip prerequisite checks for appropriate agents

## Examples

1. **Basic usage**:
   ```
   /full-workflow HIVE-2883
   ```

2. **For different component**:
   ```
   /full-workflow CCO-1234
   ```

## Arguments
- **$1** (required): JIRA issue key (e.g., HIVE-2883, CCO-1234)

## Prerequisites
- JIRA MCP configured and accessible
- GitHub CLI (`gh`) installed and authenticated
- OpenShift cluster kubeconfig available (for test execution)
- Fork of openshift-tests-private configured

## Output Structure
```
.claude/test_artifacts/{COMPONENT}/{JIRA_KEY}/
├── phases/
│   ├── test_requirements_output.md
│   ├── test_strategy.md
│   └── test_case_design.md
├── test_cases/
│   └── {JIRA_KEY}_test_cases.md
├── test_coverage_matrix.md
├── test_report.md
└── workflow_execution_report.md
```

## Complete Workflow
This IS the complete workflow. For step-by-step control:
1. `/generate-test-case JIRA_KEY`
2. `/generate-e2e-case JIRA_KEY`
3. `/run-tests JIRA_KEY`
4. `/submit-pr JIRA_KEY`

## See Also
- `/generate-test-case` - Manual test case generation
- `/generate-e2e-case` - Manual E2E generation
- `/run-tests` - Manual test execution

---

Execute complete test generation workflow for: **{args}**
