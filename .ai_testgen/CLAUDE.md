# CLAUDE.md

## MANDATORY Protocol

**CRITICAL: Each user request must be treated as a fresh execution: always re-read the YAML config and execute every step in full, regardless of conversation context.**  
**CRITICAL: NEVER use Task tool for agents** 

Before executing ANY user request, Claude MUST:
1. Read prompts/STARTUP_CHECKLIST.md  
2. Identify the correct agent YAML config file
3. Read the agent YAML config directly using Read tool
4. Execute EACH step in the agent configuration manually using available tools
5. Verify each step's output before proceeding to next step

**MANDATORY** = Required, cannot be skipped

---

## Fresh Start Enforcement

**CRITICAL RULE: Every user request is a fresh execution, even if it happens in the SAME conversation.**

- MUST always re-read the correct agent YAML config before executing any agent.  
- MUST NOT reuse execution context or skip steps based on conversation history.  
- MUST verify that the "Read agent YAML" step has been executed before proceeding.  
- If the YAML has not been read in the current request → STOP and re-execute mandatory steps.  

### Implementation Notes
- Introduce a request-level hook: each user request = enforce mandatory startup protocol.  
- NEVER rely on Task Tool (causes step skipping and incomplete execution).  
- Log confirmation before agent execution:  
  `Agent config loaded: config/agents/<agent>.yaml`  

---

## Why NOT to use Task Tool
- Task tool uses general-purpose subagent that doesn't follow YAML configs exactly
- Steps get skipped, simplified, or executed incorrectly
- Results in incomplete or wrong outputs
- Agent configurations are designed for direct execution only

## Supported Agents

### Agent Execution Order
The agents can be executed independently or in sequence:

**Complete End-to-End Flow:**
1. Generate Test Cases → 2. Generate E2E Test Code → 2.1 Execute E2E Tests → 3. Submit E2E Pull Request → 4. Update JIRA QE Comment

**CRITICAL: When user requests complete test generation and execution tests:**
- MUST execute test_case_generation → e2e_test_generation_openshift_private → test-executor in sequence
- Each agent uses its own configuration
- Each agent executes ALL steps defined in its config
- NO shortcuts or step skipping allowed

**Dependencies:**
- e2e_test_generation_openshift_private requires completion of test_case_generation (test case must exist before E2E generation)
- test-executor requires completion of e2e_test_generation_openshift_private
- PR submission requires completion of e2e_test_generation_openshift_private (and optionally test-executor)
- JIRA QE comment update can be executed after any agent completion

### 1. Generate Test Cases
- **Input**: Create test case for JIRA issue key (e.g., "Create test case for HIVE-2883")
- **Action**: NEVER use Task tool
- **PREREQUISITE CHECK**: Check if test case already exists:
  1. Check if test case file exists in `test_artifacts/{COMPONENT}/{JIRA_KEY}/test_cases/{JIRA_KEY}_test_case.md`
  2. If test case exists: Report existing test case location and skip generation
  3. If test case does not exist: Proceed with test case generation
- **Execution Steps:**
  1. Read `config/agents/test_case_generation.yaml`
  2. Execute each step in `task.steps` section manually
  3. Use tools: jira-mcp-snowflake MCP, WebFetch, Read, Write
  4. Verify outputs match `output.files` specifications
- **Agent config**: `config/agents/test_case_generation.yaml`
- **Output**: Test cases in markdown format for Polarion integration

### 2. Generate E2E Test Code
- **Input**: Request to generate E2E code for JIRA issue key (e.g., "generate E2E test case for HIVE-2883")
- **Action**: NEVER use Task tool
- **PREREQUISITE CHECK**: Before generating E2E test code, MUST verify test case exists:
  1. Only check if test case file exists in `test_artifacts/{COMPONENT}/{JIRA_KEY}/test_cases/{JIRA_KEY}_test_case.md`
  2. If test case does NOT exist, FIRST execute test_case_generation agent
  3. Only proceed with E2E generation after test case is confirmed to exist
- **Execution Steps:**
  1. Read `config/agents/e2e_test_generation_openshift_private.yaml`  
  2. Read and analyze the generated test case from `test_artifacts/{COMPONENT}/{JIRA_KEY}/test_cases/{JIRA_KEY}_test_case.md`
  3. Execute each subtask in `task.subtasks` section sequentially
  4. Each subtask has its own steps and success_criteria
  5. Use tools: Bash, Read, Write, Edit, WebFetch
  6. Verify each subtask's success_criteria before proceeding
- **Agent config**: `config/agents/e2e_test_generation_openshift_private.yaml`
- **Output**: E2E test code integrated into openshift-tests-private repository

### 2.1 Execute E2E Tests
- **Input**: Run the generated E2E test cases and capture their outcomes
- **Action**: NEVER use Task tool
- **Execution Steps:**
  1. Read `config/agents/test-executor.yaml`  
  2. Execute each subtask in `task.subtasks` section sequentially
  3. Each subtask has its own steps and success_criteria
  4. Use tools: Bash, Read, Write, Edit, WebFetch
  5. Capture test results and logs
  6. **Generate comprehensive test report** including test case coverage analysis and E2E execution results after successful test completion
- **Agent config**: `config/agents/test-executor.yaml`
- **Output**: Test execution results and comprehensive test reports

### 3. Submit E2E Pull Request
- **Input**: Request to create PR for generated E2E tests
- **Action**: NEVER use Task tool
- **Execution Steps:**
  1. Read `config/agents/e2e_test_generation_openshift_private.yaml` to identify where E2E test code was generated
  2. Navigate to the E2E test code location and verify test files exist
  3. Load and apply PR submission rules from `config/rules/pr_submission_rules.yaml`
  4. Use GitHub CLI (gh) to create pull request with hold status
- **Tools**: Bash (git, gh CLI), Read, Write
- **Output**: Created pull request with E2E test code and JIRA updated

### 4. Update JIRA QE Comment [In Progress]
- **Input**: Request to add QE comment to update QE test status

## Agent Usage Examples

**CRITICAL: When a user requests to "generate test cases and run them", Claude MUST execute test_case_generation → e2e_test_generation_openshift_private → test-executor sequentially, performing all steps and verifying outputs at each stage, even if this request occurs in the same conversation. DO NOT STOP ANY STEPS.**

### MANDATORY STEP-BY-STEP EXECUTION PROTOCOL

**BEFORE STARTING ANY AGENT:**
1. **Read agent YAML config completely** - Use Read tool to load entire config file
2. **Execute steps in EXACT sequence** - Must complete step N before starting step N+1
3. **Verify each step's output** - Confirm step completion before proceeding to next step
4. **Use EXACT commands from config** - Copy commands exactly as written in agent YAML
5. **NEVER skip, simplify, or combine steps** - Each step must be executed individually

**VIOLATION PREVENTION RULES:**
- If ANY step is skipped → STOP immediately and restart from skipped step
- Each subtask completion MUST verify ALL steps executed before proceeding to next subtask
- If using different commands than specified in config → VIOLATION, must use exact config commands
- If combining multiple steps into one action → VIOLATION, must execute separately


### Example: Complete Flow for HIVE-2883
```
User: "Create test case for HIVE-2883"
→ Execute test_case_generation agent

User: "Generate E2E code for HIVE-2883" 
→ Execute e2e_test_generation_openshift_private agent

User: "Run the E2E tests for HIVE-2883"
→ Execute test-executor agent

User: "Generate test cases and run them for HIVE-2883"
→ Execute test_case_generation → e2e_test_generation_openshift_private → test-executor

User: "Create a Pull Request for HIVE-2883 E2E tests"
→ Execute PR submission process

User: "Add QE comment to HIVE-2883"
→ Execute JIRA QE comment update process
```


## Key Rules
- Always read agent YAML configs directly
- Execute each step manually using available tools
- Verify each step before proceeding
- Follow agent configurations exactly

