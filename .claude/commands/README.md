# Slash Commands

## Command Reference

All commands follow the format: `/command-name JIRA_KEY`

### Core Workflow Commands

| Command | Description | Duration |
|---------|-------------|----------|
| `/generate-test-case` | Generate test cases from JIRA | ~90s |
| `/generate-e2e-case` | Generate E2E test code | ~120s |
| `/run-tests` | Execute E2E tests | ~180s |
| `/generate-report` | Generate comprehensive report | ~45s |
| `/submit-pr` | Create pull request | ~30s |
| `/full-workflow` | Execute complete workflow | ~3-4min |

### Regeneration Commands

| Command | Description |
|---------|-------------|
| `/regenerate-test` | Force regenerate test case |
| `/regenerate-e2e` | Force regenerate E2E code |

## Command Flow

```
/generate-test-case  →  /generate-e2e-case  →  /run-tests  →  /submit-pr
      (90s)                    (120s)             (180s)          (30s)
                                    ↓
                         /generate-report
                              (45s)
                                    
Complete workflow: /full-workflow (combines first 3 steps)
```

## Quick Start

### Complete Workflow (Recommended)
```bash
/full-workflow HIVE-2883
```

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

### Regeneration
```bash
# Force regenerate test case
/regenerate-test HIVE-2883

# Force regenerate E2E code
/regenerate-e2e HIVE-2883
```

## Prerequisites

### Required Tools
- JIRA MCP configured and accessible
- GitHub CLI (`gh`) installed and authenticated
- OpenShift cluster kubeconfig available
- Fork of openshift-tests-private configured

### Environment Setup
Ensure your environment meets the requirements listed in each command's documentation.

## Output Structure

All artifacts are generated in `.claude/test_artifacts/{COMPONENT}/{JIRA_KEY}/`:

```
.claude/test_artifacts/hive/HIVE-2883/
├── phases/
│   ├── test_requirements_output.md
│   ├── test_strategy.md
│   └── comprehensive_test_results.md
├── test_cases/
│   └── HIVE-2883_test_cases.md
├── test_coverage_matrix.md
└── test_report.md
```

E2E code is generated in `.claude/test_artifacts/{COMPONENT}/{JIRA_KEY}/openshift-tests-private/`:

```
.claude/test_artifacts/hive/HIVE-2883/openshift-tests-private/
└── test/extended/hive/
    └── hive_2883_test.go
```

## Command Details

Each command is documented with:
- **Name**: Command identifier
- **Synopsis**: Usage syntax
- **Description**: What the command does
- **Implementation**: Which agent it executes
- **Return Value**: Success/failure outputs
- **Examples**: Usage examples
- **Arguments**: Required parameters
- **Prerequisites**: Dependencies
- **See Also**: Related commands

Use `/command-name` to access detailed documentation for each command.

## Workflow Orchestrator

The workflow orchestrator (`config/agents/workflow_orchestrator.md`) manages:
- Trigger keyword matching
- Prerequisite validation
- Agent execution sequencing
- Error handling and recovery

## Related Documentation

- **Agent Configurations**: `.claude/config/agents/`
- **Rules**: `.claude/config/rules/`
- **Templates**: `.claude/config/templates/`
- **Workflow Guide**: `WORKFLOW_ORCHESTRATOR_GUIDE.md`
- **Startup Checklist**: `STARTUP_CHECKLIST.md`
