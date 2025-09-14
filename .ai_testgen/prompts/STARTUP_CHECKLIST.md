# Startup Checklist

## MANDATORY Steps

### Before Processing User Request
- [ ] Read CLAUDE.md file first
- [ ] Identify request type and workflow:
  - "Create test case for JIRA-XXX" → Workflow 1 (Test Case Generation)
  - "Generate E2E code for JIRA-XXX" → Workflow 2 (E2E Code Generation)
  - "Run E2E tests for JIRA-XXX" → Workflow 2.1 (E2E Test Execution)
  - "Create E2E PR for JIRA-XXX" → Workflow 3 (Submit E2E PR)
  - "Update test case to Polarion for JIRA-XXX" → Workflow 4 (Write to Polarion)
  - "Add QE comment to JIRA-XXX, update test status" → Workflow 5 (Update JIRA QE Comment)
- [ ] Extract JIRA issue key from user input

### For ALL Workflows (Critical Rules)
- [ ] **NEVER use Task tool for agent workflows**
- [ ] **ALWAYS read agent YAML config directly using Read tool**
- [ ] **Execute EACH step in agent config manually using available tools**
- [ ] **Verify each step's output before proceeding to next step**
- [ ] **Follow agent config instructions exactly - no skipping or simplifying**

## Execution Rules
1. **Read agent YAML config directly** - Use Read tool, never Task tool
2. **Execute steps manually** - Follow config steps exactly, verify each step
3. **Use correct directory structure** - workflow_outputs/{COMPONENT}/{JIRA_KEY}/