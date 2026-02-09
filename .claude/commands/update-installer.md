Re-vendor the OpenShift installer to a SPECIFIC version you provide and verify all changes.

**USAGE**: `/update-installer <version>`

Where `<version>` is any valid Go module version specifier:
- A semantic version tag: `v0.16.1`
- A pseudo-version: `v0.0.0-20250118123456-abcdef123456`
- A commit hash: `abcdef123456` (Go will convert to pseudo-version)
- A branch name: `master` (not recommended for production)

**IMPORTANT**: You MUST provide a version. There is NO default version.

This is a COMPLEX multi-step process that can fail in many ways due to Go module dependency resolution. This command handles the common pitfalls and infinite loops that can occur.

## Overview

This command will:
1. Pre-flight checks (git status, current state)
2. Update github.com/openshift/installer to the EXACT version you specify
3. Run vendor/modfix convergence loop (with loop detection)
4. Run full verification suite
5. Report any issues requiring manual intervention

## ⚠️ Known Pitfalls & Loop Detection

The vendor→modfix→vendor→modfix cycle can loop infinitely because:
- `go mod tidy` in root picks versions independently from `go mod tidy` in apis/
- `modfix` forces apis/ to use root's versions for shared deps
- `go mod tidy` in apis/ might "helpfully" pick different versions again
- Replace/exclude directives don't auto-sync between go.mod files

This command detects and breaks these loops automatically.

## Steps

### Step 0: Validate Version Parameter

**CRITICAL FIRST STEP**: Verify that a version was provided.

If no version was provided in the command arguments, STOP and ask the user:
"What version of the OpenShift installer do you want to update to? Please provide a version (tag, commit hash, or pseudo-version)."

DO NOT proceed without a version. DO NOT default to @latest or any other version.

### Step 1: Pre-flight Checks

First, check git status to see current state:
```bash
git status --short
```

**IMPORTANT**: Note any existing uncommitted changes. The update will modify:
- `go.mod` (root)
- `go.sum` (root)
- `apis/go.mod`
- `apis/go.sum`
- `vendor/` directory
- Potentially generated code in `pkg/client/`, `apis/`, `config/crds/`

If you want to isolate the installer update changes, commit or stash other changes first.

### Step 2: Create Checkpoint

Create a checkpoint before starting (so you can rollback if needed):
```bash
git stash push -m "checkpoint-before-installer-update"
```

### Step 3: Update Installer Dependency

**CRITICAL**: Extract the version from the command arguments. The version MUST be provided by the user.

Update the installer to the EXACT version specified:
```bash
# Use the version provided in the command arguments
# DO NOT default to @latest or any other version
# DO NOT try to guess or infer what version to use
go get github.com/openshift/installer@<VERSION_FROM_COMMAND_ARGS>
```

**Important notes:**
- If the user provides a commit hash (e.g., `abcdef123456`), Go will automatically convert it to a pseudo-version
- If the user provides a tag (e.g., `v0.16.1`), use it exactly as provided
- If the user provides a pseudo-version, use it exactly as provided
- After running `go mod tidy`, Go may rewrite commit hashes to pseudo-versions in go.mod - this is expected

**Check**: Verify the installer was actually updated to the requested version:
```bash
grep 'github.com/openshift/installer' go.mod
```

### Step 4: Convergence Loop (vendor + modfix)

The vendor and modfix targets can create a ping-pong loop. We need to run them iteratively until they converge (no more changes).

**Initialize loop detection:**
```bash
# Create checksums directory for loop detection
mkdir -p /tmp/hive-update-checksums
iteration=0
max_iterations=5
```

**Run convergence loop:**
```bash
while [ $iteration -lt $max_iterations ]; do
  echo "=== Iteration $((iteration + 1)) of $max_iterations ==="

  # Capture state before vendor
  md5sum go.mod go.sum apis/go.mod apis/go.sum > /tmp/hive-update-checksums/before_${iteration}.txt 2>/dev/null || true

  echo "Running: make vendor"
  if ! make vendor; then
    echo "ERROR: make vendor failed at iteration $((iteration + 1))"
    exit 1
  fi

  echo "Running: make modfix"
  if ! make modfix; then
    echo "ERROR: make modfix failed at iteration $((iteration + 1))"
    exit 1
  fi

  # Capture state after modfix
  md5sum go.mod go.sum apis/go.mod apis/go.sum > /tmp/hive-update-checksums/after_${iteration}.txt 2>/dev/null || true

  # Check if anything changed
  if [ $iteration -gt 0 ]; then
    if diff -q /tmp/hive-update-checksums/before_${iteration}.txt /tmp/hive-update-checksums/after_${iteration}.txt > /dev/null 2>&1; then
      echo "✓ Converged after $((iteration + 1)) iterations - no more changes to go.mod files"
      break
    else
      echo "Changes detected, running another iteration..."
    fi
  fi

  iteration=$((iteration + 1))

  if [ $iteration -eq $max_iterations ]; then
    echo "⚠️  WARNING: Hit maximum iterations ($max_iterations) without convergence!"
    echo "This suggests a vendor/modfix ping-pong loop."
    echo ""
    echo "Common causes:"
    echo "1. Root and apis/ go.mod have conflicting version constraints"
    echo "2. Replace directives differ between root and apis/ (modfix doesn't sync these)"
    echo "3. Exclude directives differ between root and apis/ (modfix doesn't sync these)"
    echo ""
    echo "Next steps:"
    echo "1. Run: make modcheck"
    echo "2. Check for 'XX replace' or 'XX exclude' messages"
    echo "3. Manually sync replace/exclude directives from go.mod to apis/go.mod"
    echo "4. Check 'go mod graph' to understand version selection"
    echo ""
    echo "Continuing to verify step anyway..."
  fi
done

# Cleanup
rm -rf /tmp/hive-update-checksums
```

**What this does:**
- Runs `make vendor` (which runs `go mod tidy` + `go mod vendor` for root and apis/)
- Runs `make modfix` (which syncs shared dependency versions from root → apis/)
- Compares checksums before/after to detect when we've converged
- Breaks out early if no changes detected
- Fails if we hit max iterations (likely infinite loop)

### Step 5: Check Module Sync Status

Before running verify, check if go.mod files are in sync:
```bash
make modcheck
```

**Expected output**: `apis/go.mod is in sync` (or `in sync (after fixing)`)

**If you see errors like:**
```
XX require some/package: root(v1.2.3) apis(v1.2.4)
XX replace some/package: root(...) apis(...)
XX exclude some/package: root(v1.0.0) apis(v1.0.1)
```

These indicate mismatches that modfix couldn't auto-fix:
- `XX require` - Should be fixed by modfix, if still present something is wrong
- `XX replace` - modfix does NOT fix replace directives - you must manually sync
- `XX exclude` - modfix does NOT fix exclude directives - you must manually sync

**Manual fix for replace/exclude mismatches:**
1. Look at the root `go.mod` replace/exclude directives
2. Copy them to `apis/go.mod`
3. Run `make vendor` again
4. Run `make modfix` again
5. Run `make modcheck` again to verify

### Step 6: Run Full Verification

Now run the full verification suite:
```bash
make verify
```

**This runs multiple checks:**
- `verify-crd` - Regenerates CRDs and checks for diffs
- `verify-codegen` - Regenerates client code and checks for diffs
- `verify-vendor` - Checks vendor/ directory has no uncommitted diffs
- `modcheck` - Checks go.mod files are in sync
- `verify-govet` - Runs go vet on all packages
- `verify-imports` - Checks import naming conventions
- `verify-lint` - Runs golint
- `verify-app-sre-template` - Checks generated templates

**Common verification failures after installer updates:**

#### A) verify-crd or verify-codegen fails
**Symptom**: "git diff --exit-code pkg/client" or "git diff --exit-code apis" fails

**Cause**: The installer update changed API types, causing different generated code

**Fix**: This is expected! The generated files need to be committed:
```bash
# These changes are intentional and should be committed
git add pkg/client/ apis/ config/crds/
```

#### B) verify-vendor fails
**Symptom**: "git diff --exit-code vendor/" fails

**Cause**: vendor/ directory has uncommitted changes

**Fix**: Check what changed:
```bash
git diff vendor/ | head -50
```

If changes look reasonable (updated dependencies), commit them:
```bash
git add vendor/
```

If changes include unexpected files or look wrong, investigate further.

#### C) modcheck fails (even after convergence loop)
**Symptom**: "apis/go.mod is out of sync" with XX replace or XX exclude

**Cause**: Replace/exclude directives differ between go.mod files

**Fix**: See "Manual fix for replace/exclude mismatches" in Step 5

#### D) Build failures with new API versions
**Symptom**: Code compilation errors about missing methods, changed signatures, etc.

**Cause**: The installer update pulled in breaking changes from dependencies (often k8s.io/*, openshift/api, etc.)

**Fix**: This requires code changes:
1. Read the error messages carefully
2. Check the dependency's changelog/release notes
3. Update hive code to match new APIs
4. Common culprits:
   - `k8s.io/client-go` - frequent breaking changes
   - `k8s.io/apimachinery` - type signature changes
   - `github.com/openshift/api` - API version changes
   - `sigs.k8s.io/controller-runtime` - controller interface changes

#### E) Test failures
**Symptom**: Unit tests fail after update

**Cause**: Mock objects or test fixtures need updating for new API versions

**Fix**:
```bash
# Run tests to see failures
make test-unit

# Update mocks if needed
make generate

# Run tests again
make test-unit
```

### Step 7: Check for Transitive Dependency Conflicts

If you're seeing weird version resolution issues, inspect the dependency graph:

```bash
# See why a specific version was chosen
go mod why github.com/some/problematic-package

# See full dependency graph (warning: very long output)
go mod graph | grep problematic-package

# See what's requiring a specific package
go mod graph | grep "github.com/some/problematic-package"
```

**Common conflict resolution strategies:**

1. **Explicit version constraint**: Add a require in go.mod for the version you want
2. **Replace directive**: Force a specific version with a replace directive
3. **Exclude directive**: Exclude known-broken versions
4. **Update other deps**: Sometimes you need to update other packages to compatible versions

### Step 8: Final Status Check

After all verification passes, check final status:

```bash
git status
```

**Expected changes:**
- `go.mod` - Updated installer version + transitive deps
- `go.sum` - Updated checksums
- `apis/go.mod` - Updated shared dependency versions
- `apis/go.sum` - Updated checksums
- `vendor/` - Updated vendored code
- Possibly `pkg/client/` - Regenerated client code
- Possibly `apis/` - Regenerated deepcopy code
- Possibly `config/crds/` - Regenerated CRD yaml

### Step 9: Create Commit

If everything passes, create a commit:

```bash
git add go.mod go.sum apis/go.mod apis/go.sum vendor/
# Add any generated code changes if needed
git add pkg/client/ apis/ config/crds/

# Update commit message with the actual version you updated to (check go.mod)
INSTALLER_VERSION=$(grep 'github.com/openshift/installer' go.mod | awk '{print $2}')

git commit -m "Update OpenShift installer dependency to ${INSTALLER_VERSION}

Updated github.com/openshift/installer to ${INSTALLER_VERSION}.
This also updates transitive dependencies including k8s.io/* packages.

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>"
```

## 🆘 Emergency Rollback

If things go terribly wrong and you need to rollback:

```bash
# Reset all changes
git reset --hard HEAD

# Restore from checkpoint (if you created one)
git stash pop

# Or manually restore specific files
git checkout HEAD -- go.mod go.sum apis/go.mod apis/go.sum vendor/
```

## 🔍 Debugging Tips

### Check what version you're actually getting
```bash
# See installer version before update
grep 'github.com/openshift/installer' go.mod

# See available versions (semantic version tags only)
go list -m -versions github.com/openshift/installer | tr ' ' '\n' | tail -20

# See what a specific version resolves to
go list -m github.com/openshift/installer@<your-version-here>

# See what the latest tagged version is
go list -m github.com/openshift/installer@latest
```

### Understand why a version was selected
```bash
# Why is this package included?
go mod why github.com/problematic/package

# What requires this package?
go mod graph | grep 'github.com/problematic/package'

# See all versions in dependency tree
go mod graph | grep 'k8s.io/client-go' | cut -d' ' -f2 | sort -u
```

### Find replace/exclude mismatches
```bash
# Show replace directives
echo "=== ROOT go.mod replaces ===" && grep '^replace' go.mod
echo "=== APIS go.mod replaces ===" && grep '^replace' apis/go.mod

# Show exclude directives
echo "=== ROOT go.mod excludes ===" && grep '^exclude' go.mod
echo "=== APIS go.mod excludes ===" && grep '^exclude' apis/go.mod

# Compare them
diff <(grep '^replace' go.mod | sort) <(grep '^replace' apis/go.mod | sort)
diff <(grep '^exclude' go.mod | sort) <(grep '^exclude' apis/go.mod | sort)
```

## 📚 Understanding the Module Structure

Hive has a multi-module structure:
- **Root module** (`go.mod`): Main hive code
- **APIs module** (`apis/go.mod`): Shared API types

Both modules can depend on the same packages (like k8s.io/apimachinery). Go's MVS (Minimal Version Selection) will independently pick versions for each module during `go mod tidy`.

**modfix** syncs shared dependencies from root → apis, but:
- ✅ Syncs `require` directives
- ❌ Does NOT sync `replace` directives
- ❌ Does NOT sync `exclude` directives

This is why you can get into loops where modfix fixes requires, but replace/exclude stay mismatched.
