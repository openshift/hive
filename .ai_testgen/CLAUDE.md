# CLAUDE.md

## MANDATORY Protocol

Before executing ANY user request, Claude MUST:
1. Read this CLAUDE.md file first
2. Read prompts/STARTUP_CHECKLIST.md  
3. **CRITICAL: NEVER use Task tool for agents**
4. Identify the correct agent YAML config file
5. Read the agent YAML config directly using Read tool
6. Execute EACH step in the agent configuration manually using available tools
7. Verify each step's output before proceeding to next step

**MANDATORY** = Required, cannot be skipped

## Why NOT to use Task Tool
- Task tool uses general-purpose subagent that doesn't follow YAML configs exactly
- Steps get skipped, simplified, or executed incorrectly
- Results in incomplete or wrong outputs
- Agent configurations are designed for direct execution only

## Supported Workflows

### Workflow Execution Order
The workflows can be executed independently or in sequence:

**Complete End-to-End Flow:**
1. Generate Test Cases → 2. Generate E2E Test Code → 2.1 Execute E2E Tests → 3. Submit E2E Pull Request → 4. Update Polarion → 5. Update JIRA QE Comment

**Dependencies:**
- Workflow 2.1 requires completion of Workflow 2
- Workflow 3 requires completion of Workflow 2 (and optionally 2.1)
- Workflow 4 requires completion of Workflow 1
- Workflow 5 can be executed after any workflow completion

### 1. Generate Test Cases
- **Input**: Create test case for JIRA issue key (e.g., "Create test case for HIVE-2883")
- **Action**: NEVER use Task tool
- **Execution Steps:**
  1. Read `config/agents/test_case_generation.yaml` using Read tool
  2. Execute each step in `task.steps` section manually
  3. Use tools: jira-mcp-snowflake MCP, WebFetch, Read, Write
  4. Verify outputs match `output.files` specifications
- **Agent config**: `config/agents/test_case_generation.yaml`
- **Output**: Test cases in markdown format for Polarion integration

### 2. Generate E2E Test Code
- **Input**: Request to generate E2E code for JIRA issue key (e.g., "generate E2E test case for HIVE-2883")
- **Action**: NEVER use Task tool
- **Execution Steps:**
  1. Read `config/agents/e2e_test_generation_openshift_private.yaml` using Read tool
  2. Execute each subtask in `task.subtasks` section sequentially
  3. Each subtask has its own steps and success_criteria
  4. Use tools: Bash, Read, Write, Edit, WebFetch
  5. Verify each subtask's success_criteria before proceeding
- **Agent config**: `config/agents/e2e_test_generation_openshift_private.yaml`
- **Output**: E2E test code integrated into openshift-tests-private repository

### 2.1 Execute E2E Tests
- **Input**: Run the generated E2E test cases and capture their outcomes
- **Action**: NEVER use Task tool
- **Execution Steps:**
  1. Read `config/agents/test-executor.yaml` using Read tool
  2. Execute each subtask in `task.subtasks` section sequentially
  3. Each subtask has its own steps and success_criteria
  4. Use tools: Bash, Read, Write, Edit, WebFetch
  5. Capture test results and logs
- **Agent config**: `config/agents/test-executor.yaml`
- **Output**: Test execution results and reports

### 3. Submit E2E Pull Request
- **Input**: Request to create PR for generated E2E tests
- **Action**: NEVER use Task tool
- **Tools**: Bash (git, gh CLI), Read, Write, jira-mcp-snowflake MCP
- **Output**: Created pull request with E2E test code and JIRA updated

### 4. Update Polarion [In Progress]
- **Input**: Request to update Polarion with test case information
- **Action**: NEVER use Task tool
- **Execution Steps:**
  1. Read generated test case files
  2. Format according to Polarion requirements
  3. Use Polarion API integration tools
  4. Upload/update test cases in Polarion system
- **Tools**: Read, Write, WebFetch (for API calls)
- **Output**: Updated Polarion test cases

### 5. Update JIRA QE Comment [In Progress]
- **Input**: Request to add QE comment to update QE test status

## Workflow Usage Examples

### Example 1: Complete Flow for HIVE-2883
```
User: "Create test case for HIVE-2883"
→ Execute Workflow 1

User: "Generate E2E code for HIVE-2883" 
→ Execute Workflow 2

User: "Run the E2E tests for HIVE-2883"
→ Execute Workflow 2.1

User: "Submit PR for HIVE-2883 E2E tests"
→ Execute Workflow 3

User: "Update Polarion with HIVE-2883 test cases"
→ Execute Workflow 4

User: "Add QE comment to HIVE-2883"
→ Execute Workflow 5
```

### Example 2: Individual Workflows
```
User: "Just generate test cases for CCO-1234"
→ Execute only Workflow 1

User: "Create E2E tests for HIVE-5678 and submit PR"
→ Execute Workflow 2 → Workflow 3
```

## Key Rules
- Always read agent YAML configs directly
- Execute each step manually using available tools
- Verify each step before proceeding
- Follow agent configurations exactly
