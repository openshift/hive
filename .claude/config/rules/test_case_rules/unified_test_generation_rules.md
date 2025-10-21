# Unified Test Case Generation Framework

**Integrated**: Core Problem → Thinking Phases → Architecture → Validation → Quality

---

## 1. CORE PROBLEM IDENTIFICATION (EXECUTE FIRST)

### PRIMARY RULE
Identify single technical root cause, design minimal validation

### MANDATORY
- Multiple platforms = 1 test case with platform note
- **ORDER**: Core Problem → Thinking Phases → Test Generation

---

## 2. THINKING PHASES (MANDATORY)

### FAST MODE
Skip Phase 2 if issue has clear root cause + standard validation pattern

### PHASE 1: ECOSYSTEM CONTEXT (MANDATORY)

**Questions:**
- How do real users encounter this issue?
- User-facing component or part of larger workflow?
- What user action triggers affected resources?

**Analysis:**
- Map JIRA technical components → user-facing operations
- Distinguish internal components vs user-accessible APIs
- Identify realistic scenarios exposing bug in production

**Output:** Complete mapping: technical component → user workflow

---

### PHASE 2: USER REALITY (FAST MODE CAN SKIP)

**Questions:**
- Realistic operational context?
- User scale and complexity?
- Environmental constraints and dependencies?

**Analysis:**
- Target users: engineers, developers, operators, admins
- Scale: cluster count, resource volume, operation frequency
- Common patterns, error scenarios, recovery procedures

**Output:** Test scenarios mirroring authentic user environments

---

### PHASE 3: SYSTEM INTEGRATION (MANDATORY)

**Questions:**
- Component integration with other components?
- Data flow and propagation mechanisms?
- Configuration flow: user input → final state?

**Analysis:**
- All components in user workflow
- Data/config propagation, dependency chains
- Failure patterns, error handling

**Architecture Sources:**
- PR analysis (gh CLI/WebFetch) for code changes
- DeepWiki for stable knowledge (~1 week lag)
- Local repo: `grep -r ComponentName ./pkg/ ./cmd/`, `find . -name *controller*`

**Output:** Multi-component interaction understanding

---

### PHASE 4: VALUE DELIVERY (MANDATORY)

**Questions:**
- How validate complete user workflow?
- Quantitative measurements for desired outcome?

**Analysis:**
- Quantitative measurements for workflow completion
- Numerical thresholds for pass/fail
- End-to-end value delivery, not just component function

**Output:** Measurable criteria for successful functionality

---

## 3. TECHNICAL VALIDATION (UNIVERSAL)

### Core Questions

| Question | Description |
|----------|-------------|
| **What Changed?** | Specific behavior/state/output difference after fix? |
| **How to Observe?** | Observable artifact proving the change? |
| **What Threshold?** | Numerical/binary criterion for success vs failure? |

### 5 Validation Types - CHOOSE ONE

| Type | When to Use | Approach |
|------|-------------|----------|
| **A: Stability** | changing/inconsistent behavior | measure consistency across time |
| **B: Frequency** | too frequent/excessive behavior | count occurrences, compare threshold |
| **C: State** | incorrect state | compare actual vs expected |
| **D: Absence** | unwanted content | verify not found |
| **E: Progression** | stuck/not progressing | monitor state transitions |

### Meta-Rules
- ✅ Quantitative > Qualitative
- ✅ Observable > Internal
- ✅ Reproducible + Contextual + Decisive

### Pattern Selection
- ⛔ **FORBIDDEN**: Multiple patterns for same issue, 3+ test cases for single root cause
- ✅ **REQUIRED**: ONE most direct pattern for root cause

---

## 4. TEST QUALITY RULES

### ✅ Good Test Characteristics
- Realistic user workflows, not direct resource manipulation
- Specific measurable results with numerical thresholds
- Platform-appropriate CLI tools (oc, kubectl, etc.)
- Recovery scenarios: error → healthy state
- End-to-end flows, not isolated components

### ❌ Bad Test Characteristics
- Direct manipulation of internal/auto-generated resources
- Isolated component tests without user context
- Vague results, missing quantitative validation
- Inappropriate CLI tools, no recovery testing

### Mandatory Elements
- ✅ Time-based measurements for performance/behavior
- ✅ Frequency counting for reconciliation/update problems
- ✅ Log pattern analysis for component behavior
- ✅ Resource state tracking for consistency
- ✅ Metadata change tracking (resourceVersion, generation)
- ✅ Recovery path validation: error state → healthy state

### Simple Validation Checklist

| Check | Question |
|-------|----------|
| **1** | Test reproduces JIRA issue through realistic user scenarios? |
| **2** | User would naturally perform these steps in production? |
| **3** | Every technical problem has quantitative validation with metrics? |

---

## 5. ARCHITECTURE UNDERSTANDING

### Data Sources

| Source | Use Case | Details |
|--------|----------|---------|
| **PR Analysis** | Code changes, new implementations | `gh CLI` / `WebFetch` |
| **DeepWiki** | Stable knowledge | Platform reqs, architecture (~1 week lag) |
| **Local Repo** | Latest source code | `grep -r ComponentName ./pkg/ ./cmd/`<br>`find . -name *controller*` |

### Analysis Tasks
- Component role in overall system
- End-to-end flow exposing reported issue
- User workflows vs direct resource manipulation
- Component dependencies and integration points
- How users interact: directly or through higher-level operations?
- What triggers resource creation/modification in real scenarios?

### Repository Context
- **Product Code**: What we ANALYZE - project source
- **Test Code**: Where we WRITE - test repository
- **Check**: `pwd; ls -la | grep -E '(go.mod|Makefile|pkg|cmd|src)'`

---

## 6. TEST COVERAGE & GENERATION

### Test Coverage Focus
- **Primary**: End-to-end user workflows naturally exercising component
- **Scenarios**: Happy path, edge cases, error scenarios, integration, recovery
- **Commands**: Platform-appropriate CLI, realistic naming, executable in real environments
- **Validation**: Metadata tracking, status analysis, log analysis, event monitoring

### Test Steps Guidelines
- ✅ **Workflow-Driven**: complete user operations
- ✅ **Realistic Scenarios**: authentic interaction patterns
- ✅ **Measurable Results**: quantifiable expected results
- ✅ **Executable Commands**: exact commands users execute
- ⛔ **AVOID**: internal/auto-generated resource manipulation

---

## 7. FRAMEWORK ENFORCEMENT

### Mandatory Completion
- FAST MODE active if: clear root cause + standard validation pattern
- Complete phases in sequence before technical analysis
- Each phase produces concrete outputs

### FAST MODE Details
- **Skip**: Phase 2 (User Reality)
- **Flow**: Phase 1 → Phase 3
- **Saves**: 15-20 seconds

### Violation Handling
- Incomplete phase → **STOP**, complete missing phases
- Analysis contradicting framework → **revise**
- Test not reflecting user reality → **redesign**
- Component-isolation ignoring ecosystem → **prohibited**

---

## 8. QUANTITATIVE VALIDATION EXAMPLES

### Component Behavior
- Measure operation frequency over time window
- Compare metadata before/after (resourceVersion, generation)
- Count log entries for expected patterns
- Time-based status stability monitoring

### Error Handling
- Content comparison across operations
- Pattern match unwanted content removal
- State transition validation with timeouts
- Resource cleanup with quantitative checks

### Recovery Validation
- Monitor: error state → healthy state transition
- Verify resource functionality after recovery
- Validate error artifact cleanup
- Confirm normal operation resumption with metrics

---

## 9. PROJECT-SPECIFIC CONFIGURATION

### Note
Component-specific rules should be maintained in separate files

### Example
- `test_case_generation_rules_{component}.yaml` for component mandatory rules
- Define project-specific test environment requirements as needed

### Usage
Load this unified framework + component-specific rules together for complete guidance
