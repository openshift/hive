---
description: Suggest make targets to run before committing based on git changes
---

Analyze the current git changes and workspace state to suggest which make targets should be run before committing.

First, check the current state:
!`git status --short`
!`git diff --cached --name-only`
!`ls -1 bin/ 2>/dev/null | wc -l`

Based on the analysis, provide suggestions following these rules:

**Build Status & Targeted Build Suggestions:**
- Changes in cmd/manager/ -> `make build-manager`
- Changes in cmd/operator/ -> `make build-operator`
- Changes in cmd/hiveadmission/ -> `make build-hiveadmission`
- Changes in contrib/cmd/hiveutil/ -> `make build-hiveutil`
- Changes in pkg/ (shared code) -> `make build` (affects manager, operator, hiveadmission), optionally `make build-hiveutil`
- If bin/ is empty but no cmd/ or pkg/ changes -> Optional: `make build`, `make build-hiveutil`

**File Change Analysis:**
- Go source files (*.go) in pkg/, cmd/, or contrib/:
  - Suggest: `make test-unit`, `make verify`
  - Apply targeted build suggestions from above
  - Optional: `make lint` (if not running full `make verify`)

- API files in apis/:
  - Suggest: `make update`, `make verify-codegen`

- Submodule dependency files (apis/go.mod, apis/go.sum):
  - Suggest: `make vendor-submodules`

- CRD files in config/crds/:
  - Suggest: `make verify-crd`

- Dependency files (go.mod, go.sum):
  - Suggest: `make vendor`, `make verify-vendor`
  - Optional: `make modcheck`, `make modfix`

- Vendor directory:
  - Suggest: `make verify-vendor`

- Generated files (zz_generated.*.go):
  - Suggest: `make verify-codegen`

- Makefile or build scripts:
  - Suggest: `make build`, `make verify`

- App-SRE template files:
  - Suggest: `make verify-app-sre-template`

**Output Format:**
Present changed files, build status, recommended targets (with reasons), optional targets, and a quick command combining the main targets.
