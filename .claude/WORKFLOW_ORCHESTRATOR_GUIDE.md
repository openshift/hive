# Workflow Orchestrator Usage Guide

## Overview

The **Workflow Orchestrator** (`workflow_orchestrator.yaml`) is a top-level agent that automatically identifies user intent and orchestrates multi-agent workflows. It eliminates the need for manual workflow identification and ensures correct agent execution order.

## Key Benefits

✅ **Automatic Intent Recognition** - Parses user requests and identifies workflows  
✅ **Dependency Management** - Validates prerequisites before execution  
✅ **REGENERATE Mode Support** - Automatically handles re-creation scenarios  
✅ **Error Handling** - Unified error handling across all workflows  
✅ **Execution Reports** - Generates comprehensive workflow execution reports  

---

## Supported Workflows

### 1. **full_flow** - Complete Test Generation and Execution
**Trigger Keywords**: "generate test cases and run", "complete flow", "full test generation"

**Agents Executed**:
1. `test_case_generation` - Generate Polarion test cases
2. `e2e_test_generation_openshift_private` - Generate E2E test code
3. `test-executor` - Execute E2E tests

**Example**:
```
User: "Generate test cases and run them for HIVE-2883"
```

---

### 2. **test_case_only** - Test Case Generation Only
**Trigger Keywords**: "create test case", "generate test case", "test case for"

**Agents Executed**:
1. `test_case_generation` - Generate Polarion test cases

**Example**:
```
User: "Create test case for HIVE-2883"
```

---

### 3. **e2e_generation** - E2E Test Generation
**Trigger Keywords**: "generate e2e", "create e2e test", "e2e code for"

**Agents Executed**:
1. `e2e_test_generation_openshift_private` - Generate E2E test code (orchestrator)
   - Sub-agents: validation → setup → generation → quality check

**Example**:
```
User: "Generate E2E code for HIVE-2883"
```

---

### 4. **test_execution** - Test Execution Only
**Trigger Keywords**: "run e2e tests", "execute tests", "run tests for"

**Agents Executed**:
1. `test-executor` - Execute E2E tests and generate reports

**Example**:
```
User: "Run E2E tests for HIVE-2883"
```

---

### 5. **pr_submission** - PR Submission
**Trigger Keywords**: "create pr", "submit pr", "create pull request"

**Agents Executed**:
1. `pr-submitter` - Submit PR to openshift-tests-private

**Example**:
```
User: "Create PR for HIVE-2883"
```

---

### 6. **e2e_and_run** - E2E Generation and Execution
**Trigger Keywords**: "generate e2e and run", "create e2e and execute"

**Agents Executed**:
1. `e2e_test_generation_openshift_private` - Generate E2E test code
2. `test-executor` - Execute E2E tests

**Example**:
```
User: "Generate E2E and run for HIVE-2883"
```

---

## REGENERATE Mode

### Detection Keywords
- `"re-create"` / `"recreate"`
- `"re-generate"` / `"regenerate"`

### Behavior
When REGENERATE mode is detected:
- ✅ Skip prerequisite checks for agents with `skip_on_regenerate: true`
- ✅ Overwrite existing files without confirmation
- ✅ Force re-execution even if output already exists

### Examples
```bash
# Regenerate test case (always regenerates)
"re-create test case for HIVE-2883"

# Regenerate E2E test (skips test case prerequisite check)
"recreate e2e test for HIVE-2883"

# Normal generation (checks prerequisites)
"generate e2e test for HIVE-2883"  # ← Will fail if test case doesn't exist
```

---

## How It Works

### Execution Flow

```
┌─────────────────────────────────────┐
│ 1. Parse User Request               │
│    - Extract JIRA key               │
│    - Detect REGENERATE mode         │
│    - Convert to lowercase           │
└─────────────────────────────────────┘
                 ↓
┌─────────────────────────────────────┐
│ 2. Identify Workflow                │
│    - Match trigger keywords         │
│    - Select best matching workflow  │
└─────────────────────────────────────┘
                 ↓
┌─────────────────────────────────────┐
│ 3. Validate Prerequisites           │
│    - Check required files exist     │
│    - Skip if REGENERATE mode        │
└─────────────────────────────────────┘
                 ↓
┌─────────────────────────────────────┐
│ 4. Execute Agents in Sequence       │
│    - Read agent YAML config         │
│    - Execute all agent steps        │
│    - Verify completion              │
└─────────────────────────────────────┘
                 ↓
┌─────────────────────────────────────┐
│ 5. Generate Execution Report        │
│    - List executed agents           │
│    - List output files              │
│    - Calculate execution time       │
└─────────────────────────────────────┘
```

---

## Prerequisite Checks

Each workflow validates prerequisites before execution:

| Workflow | Agent | Prerequisite File/Directory |
|----------|-------|----------------------------|
| **full_flow** | e2e_test_generation | `test_artifacts/{COMPONENT}/{JIRA_KEY}/test_cases/{JIRA_KEY}_test_case.md` |
| **full_flow** | test-executor | `temp_repos/openshift-tests-private/test/extended/cluster_operator/{COMPONENT}/` |
| **e2e_generation** | e2e_test_generation | `test_artifacts/{COMPONENT}/{JIRA_KEY}/test_cases/{JIRA_KEY}_test_case.md` |
| **test_execution** | test-executor | `temp_repos/openshift-tests-private/test/extended/cluster_operator/{COMPONENT}/` |
| **pr_submission** | pr-submitter | `temp_repos/openshift-tests-private/test/extended/cluster_operator/{COMPONENT}/` |

**Note**: Test case generation has no prerequisites and can always run.

---

## Output Files

The orchestrator generates a comprehensive execution report:

**File**: `test_artifacts/{COMPONENT}/{JIRA_KEY}/workflow_execution_report.md`

**Contents**:
- Workflow name and description
- JIRA issue key
- REGENERATE mode status
- List of agents executed with status
- All generated output files
- Total execution time
- Any errors encountered

---

## Adding New Workflows

To add a new workflow, edit `config/agents/workflow_orchestrator.yaml`:

```yaml
workflows:
  your_new_workflow:
    name: "Your Workflow Name"
    description: "What this workflow does"
    trigger_keywords:
      - "keyword 1"
      - "keyword 2"
    agents:
      - name: agent_name
        config: "config/agents/agent_name.yaml"
        prerequisite: previous_agent_name  # or null
        prerequisite_check: "path/to/check"
        skip_on_regenerate: true  # or false
        description: "What this agent does"
```

---

## Manual Fallback

If the orchestrator cannot match a workflow (no keywords match), it will:
1. Log a warning: `⚠️ No workflow matched - falling back to manual identification`
2. Allow AI to manually identify the appropriate agent
3. Execute the selected agent directly

This ensures the system remains flexible for edge cases.

---

## Best Practices

1. **Always use the orchestrator** - Start every request by reading `workflow_orchestrator.yaml`
2. **Use clear trigger keywords** - Help the orchestrator identify the correct workflow
3. **Include JIRA key** - Always include the JIRA issue key in the request
4. **Use REGENERATE keywords** - Explicitly use "re-create" or "regenerate" when needed
5. **Check execution reports** - Review the generated workflow execution report

---

## Troubleshooting

### Problem: Workflow not matched
**Solution**: Use more specific trigger keywords from the workflow definitions

### Problem: Prerequisite check fails
**Solution**: Either run the prerequisite workflow first, or use REGENERATE mode to skip checks

### Problem: Agent execution fails
**Solution**: Check the workflow execution report for error details and recovery suggestions

---

## Integration with STARTUP_CHECKLIST.md

The orchestrator is now the **primary entry point** for all agent executions. See `STARTUP_CHECKLIST.md` for detailed execution protocols and examples.

**Key Rule**: Always read `config/agents/workflow_orchestrator.yaml` before processing any user request.

