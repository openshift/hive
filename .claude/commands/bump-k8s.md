---
description: Revendor all k8s-related dependencies to the next Y version
---

Help me revendor all Kubernetes-related dependencies. Follow this workflow:

## Phase 1: Identify and Update K8s Dependencies

1. Read go.mod to find a known k8s dependency (like k8s.io/api) and identify its current version
2. Extract the current Y version (e.g., if k8s.io/api is v0.33.3, current Y is "33")
3. Determine the next Y version (e.g., from "33" to "34")
4. Search go.mod for all k8s-related dependencies matching the current Y version
5. For each k8s-related dependency found:
   - Search for the most recent published semver tag in the **next** Y version
   - Update to the latest Z version within the **next** Y version
   - Note: Z versions can differ across dependencies due to interdependencies

6. Iteratively run `go mod tidy` and `go mod vendor` to flush out downstream dependency issues
   - **Important**: After each `go mod tidy` or any edit to go.mod, you must:
     1. Run `make modfix` (from repository root) to sync apis/go.mod
     2. `cd` into the apis/ directory and run `go mod tidy && go mod vendor`
     3. **Return to the repository root directory** (e.g., `cd /workspace`)
     4. Run `make update vendor` from the repository root
7. If critical dependencies like openshift/installer or openshift/client-go break:
   - Go to their repository
   - Find a recent tagged or released commit
   - Check their go.mod to see what k8s versions they require
   - Adjust our versions accordingly
   - **STOP** if we would need to change to a different Y version - try something different instead
   - **Note**: `github.com/openshift/client-go` is forked from `k8s.io/client-go` and should match versions
     - Check the `release-4.XX` branches in https://github.com/openshift/client-go
     - Find a branch/commit that uses the same k8s.io/client-go version you're targeting
     - Use `go get github.com/openshift/client-go@<commit-hash>` to update it

## Phase 2: Sync apis/go.mod

8. Once the root go.mod dependency graph is stable, run `make modfix` to sync apis/go.mod
9. Run `go mod tidy` and `go mod vendor` in the apis/ directory
10. Return to the repository root directory and run `make update vendor`

## Phase 3: Compile and Test

11. Run `make test` to compile and run unit tests
12. If compilation errors occur, fix them. Common issues:
    - Deprecated functions/symbols removed - update to recommended replacements
    - Function signatures changed (params added/removed) - accommodate changes
13. Keep iterating until `make test` succeeds

## Phase 4: Commit

14. When all tests pass, commit the changes

**Important**: If you get stuck at any point, stop and ask for help rather than trying to commit broken code.

Track your progress with the TodoWrite tool and work through each phase methodically.
