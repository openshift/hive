# Startup Checklist

## CRITICAL ENFORCEMENT - NO EXCEPTIONS

**ABSOLUTE REQUIREMENT: You MUST execute EVERY SINGLE STEP in agent YAML configurations. NO SKIPPING, NO COMBINING, NO SIMPLIFYING.**

## EXECUTION EFFICIENCY REQUIREMENTS (MANDATORY)

**CRITICAL: Must execute with maximum efficiency and minimal verbosity**

**OUTPUT FORMAT ENFORCEMENT**:
- Each step must have explicit output: `✅ Step X completed: [result]`
- NEVER skip step numbers (Step 1 → Step 2 → Step 3 → Step 4)
- Silent thinking execution (no verbose explanations), but clear step completion markers
- NO process descriptions or step-by-step commentary

**VIOLATION ENFORCEMENT**: If providing ANY explanatory text or verbose process descriptions, this is an EFFICIENCY VIOLATION and execution must restart with direct execution only.

## FRESH START ENFORCEMENT

**CRITICAL RULE: Every user request is a fresh execution, even if it happens in the SAME conversation.**

### What MUST Be Re-read Each Request:
- ✅ **MUST always re-read workflow_orchestrator.yaml** (user may have modified it)
- ✅ **MUST always re-read the agent YAML config** (user may have modified it)
- ✅ **MUST always re-read rule files if agent requires them** (rules may have changed)

### What CAN Be Reused Across Requests:
- ✅ **Generated artifacts** (test_artifacts/*) - unless user says "regenerate/re-create"
- ✅ **E2E test code** (temp_repos/*) - unless user says "regenerate/re-create"
- ✅ **Prerequisite validation** - orchestrator will check if artifacts exist

### Key Principle:
- **Configuration** = Always re-read (may have changed)
- **Artifacts** = Reuse if exists (unless regenerate mode)
- **Agent execution** = Follow orchestrator's prerequisite validation logic
- Each user request = enforce mandatory startup protocol

## MANDATORY Steps

### Before Processing User Request - USE WORKFLOW ORCHESTRATOR
- [ ] **MANDATORY: ALWAYS start by reading workflow_orchestrator.yaml**: Read `config/agents/workflow_orchestrator.yaml`
- [ ] **Let orchestrator identify workflow automatically**: Use workflow definitions and trigger keywords
- [ ] **Orchestrator will automatically**:
  - Extract JIRA issue key from user request
  - Match user request against workflow trigger keywords
  - Detect REGENERATE mode ("re-create", "recreate", "re-generate", "regenerate")
  - Validate prerequisites before execution
  - Execute agents in correct sequence
  - Generate workflow execution report

### Manual Agent Identification (If Orchestrator Cannot Match)
If workflow_orchestrator cannot identify a workflow, fallback to manual identification:
- [ ] Identify request type and agent to execute:
  - "Create test case for JIRA-XXX" → `test_case_generation` agent
  - "Generate E2E code/test for JIRA-XXX" → `e2e_test_generation_openshift_private` agent
  - "Run E2E tests for JIRA-XXX" → `test-executor` agent
  - "Create E2E PR for JIRA-XXX" → `pr-submitter` agent

### For ALL Agent Executions (Critical Rules)
- [ ] **NEVER use Task tool for agent workflows**
- [ ] **ALWAYS read agent YAML config directly using Read tool**
- [ ] **Execute EACH step in agent config manually using available tools**
- [ ] **Verify each step's output before proceeding to next step**
- [ ] **Follow agent config instructions exactly - no skipping or simplifying**

### CRITICAL: Rule Keyword Enforcement (MANDATORY)
**When reading any rule files, configuration files, or guideline documents:**
- [ ] **MANDATORY keywords = ABSOLUTE REQUIREMENT** - Must be executed without exception
- [ ] **NEVER keywords = ABSOLUTE PROHIBITION** - Must never be violated under any circumstances
- [ ] **CRITICAL keywords = STOP and verify** - Must pause and verify compliance before proceeding
- [ ] **FORBIDDEN keywords = ABSOLUTE PROHIBITION** - Same as NEVER, must not be violated
- [ ] If ANY of these keywords are violated → **STOP immediately and restart from beginning**

**Enforcement Rules:**
- When encountering "MANDATORY: Do X" → X must be done, no alternatives
- When encountering "NEVER do Y" → Y is absolutely prohibited, no exceptions
- When encountering "CRITICAL: Check Z" → Must verify Z before any further action
- When encountering "FORBIDDEN: Action W" → Action W is absolutely prohibited
- Violation of any keyword rule = **Complete execution failure, must restart**

## STRICT EXECUTION PROTOCOL

### Step-by-Step Execution Requirements
1. **Read agent YAML config directly** - Use Read tool, never Task tool
2. **Execute steps manually** - Follow config steps exactly, verify each step
3. **Use correct directory structure** - test_artifacts/{COMPONENT}/{JIRA_KEY}/
4. **Parallel execution mandatory** - Execute independent tool calls in parallel within same message

### MANDATORY VERIFICATION FOR EACH STEP
- [ ] Execute step exactly as written in YAML config
- [ ] Verify step output/result
- [ ] Log completion message: `✅ Step X completed: [brief result]`
- [ ] Show step output/result (concise format)
- [ ] Confirm step meets success criteria
- [ ] Only then proceed to next step

### VIOLATION DETECTION
- [ ] If ANY step is skipped → STOP and restart from beginning
- [ ] If steps are combined → STOP and restart from beginning  
- [ ] If step verification is missing → STOP and restart from beginning
- [ ] If different commands are used → STOP and restart from beginning
- [ ] If verbose explanations provided → EFFICIENCY VIOLATION, restart

## Execution Output Format

### MANDATORY OUTPUT FORMAT FOR ALL AGENTS:
```
Step 1: Reading agent config
✅ Step 1 completed: Agent config loaded from config/agents/<agent>.yaml

Step 2: [Action from YAML step 1]
✅ Step 2 completed: [Brief result]

Step 3: [Action from YAML step 2]
✅ Step 3 completed: [Brief result]

[Continue for ALL steps in the YAML config...]
```

### EFFICIENCY RULES:
- **60-70% fewer tokens than verbose mode**
- **No process descriptions** - only step completions
- **No explanatory text** - only execution results
- **Brief result summaries** - 1-2 sentences maximum per step
- **Clear step numbers** - never skip sequence

## Agent Execution Examples (Using Workflow Orchestrator)

### Example 1: Complete Flow (Recommended Approach)
```
User: "Generate test cases and run them for HIVE-2883"

Step 1: Read workflow_orchestrator.yaml
✅ Step 1 completed: Orchestrator config loaded

Step 2: Parse user request
✅ Step 2 completed: JIRA key=HIVE-2883, Workflow=full_flow, Regenerate=false

Step 3: Validate prerequisites
✅ Step 3 completed: All prerequisites validated

Step 4: Execute workflow agents
  4.1: Executing test_case_generation agent
       ✅ Agent completed: 4 files generated
  4.2: Executing e2e_test_generation_openshift_private agent
       ✅ Agent completed: E2E code integrated
  4.3: Executing test-executor agent
       ✅ Agent completed: Tests executed

Step 5: Generate workflow report
✅ Workflow completed: full_flow for HIVE-2883 (3 agents executed)
```

### Example 2: Single Agent via Orchestrator
```
User: "Create test case for HIVE-2883"

Step 1: Read workflow_orchestrator.yaml
✅ Step 1 completed: Orchestrator config loaded

Step 2: Parse user request
✅ Step 2 completed: Workflow identified=test_case_only

Step 3: Execute workflow
✅ Agent test_case_generation completed

Output: 4 files in test_artifacts/hive/HIVE-2883/
```

### Example 3: Regenerate Mode via Orchestrator
```
User: "re-create e2e test for HIVE-2883"

Step 1: Read workflow_orchestrator.yaml
✅ Step 1 completed: Orchestrator config loaded

Step 2: Parse user request
✅ Step 2 completed: REGENERATE mode detected, Workflow=e2e_generation

Step 3: Validate prerequisites
⚠️ REGENERATE mode - skipping prerequisite checks

Step 4: Execute workflow
✅ Agent e2e_test_generation_openshift_private completed (overwrite mode)
```

### Example 4: E2E Test Generation (Shows Sub-Agent Orchestration)
```
User: "Generate E2E code for HIVE-2883"

Step 1: Read workflow_orchestrator.yaml
✅ Workflow identified: e2e_generation

Step 2: Execute e2e_test_generation_openshift_private agent
  → This agent is also an orchestrator, executing 4 sub-agents:
  2.1: e2e_validation_agent
  2.2: repository_setup_agent  
  2.3: e2e_code_generation_agent
  2.4: e2e_quality_check_agent
✅ E2E generation workflow completed
```

### Example 5: PR Submission via Orchestrator
```
User: "Create PR for HIVE-2883"

Step 1: Read workflow_orchestrator.yaml
✅ Workflow identified: pr_submission

Step 2: Validate prerequisites
✅ E2E code exists in openshift-tests-private

Step 3: Execute pr-submitter agent
✅ PR created and submitted

Output: GitHub PR link
```

### Example 6: Manual Fallback (Orchestrator Cannot Match)
```
User: "Do something custom with HIVE-2883"

Step 1: Read workflow_orchestrator.yaml
⚠️ No workflow matched - falling back to manual identification

Step 2: Manual agent identification
→ AI determines appropriate agent based on context
→ Execute selected agent directly
```