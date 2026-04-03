#!/bin/bash

set -euo pipefail

# Match things like 'mce-2.0' or origin/mce-2.0, capturing:
# 1: 'origin/'
# 2: '2.0' (used for the bundle semver prefix)
readonly MCE_BRANCH_RE='^([^/]+/)?mce-([[:digit:]]+\.[[:digit:]]+)$'

readonly MASTER_BRANCH_RE='^([^/]+/)?master$'

# MASTER_BRANCH_PREFIX is the prefix of the Hive version that will be constructed by
# this script if we are tracking the master branch.
# The version used for images and bundles will be:
# "v{prefix}.{number of commits}-{git hash}"
# e.g. "v1.2.3187-18827f6"
readonly MASTER_BRANCH_PREFIX="1.2"

# UNKNOWN_BRANCH_PREFIX is the fallback if we can't determine an MCE or master branch
readonly UNKNOWN_BRANCH_PREFIX="0.0"

# Global state
MODE="library"
COMMIT=""
BRANCH_NAME=""
PREFIX=""
COMMIT_COUNT=""

# Logging function
log() {
    local level="$1"
    shift
    local message="$*"

    if [[ "$MODE" == "standalone" ]]; then
        echo "$message" >&2
    else
        echo "$message"
    fi

    if [[ "$level" == "fatal" ]]; then
        exit 1
    fi
}

# Get short SHA (7 characters)
shortsha() {
    git rev-parse --short=7 "$COMMIT" 2>/dev/null
}

# Determine the X.Y semver prefix for the branch name.
#
# Returns: Two-digit semver prefix corresponding to the branch.
#   - If the branch name is of the form [remote/]mce-X.Y, this will be X.Y.
#   - If the branch name is [remote/]master, we'll use the master prefix.
#   - Otherwise, we'll use the "unknown" default.
prefix_from_branch() {
    local branch_name="$1"

    # Match MCE branch pattern
    if [[ "$branch_name" =~ $MCE_BRANCH_RE ]]; then
        echo "${BASH_REMATCH[2]}"
        return 0
    fi

    # Match master branch pattern
    if [[ "$branch_name" =~ $MASTER_BRANCH_RE ]]; then
        echo "$MASTER_BRANCH_PREFIX"
        return 0
    fi

    # Default to unknown
    echo "$UNKNOWN_BRANCH_PREFIX"
}

# The user may have specified a branch name for which we don't have a local ref
# (e.g. `mce-2.1`). Rather than forcing them to specify a qualified ref, we'll
# automatically look for it in the `upstream` and `origin` remotes as well.
find_branch() {
    local branch_name="$1"
    local remote

    for remote in "" "upstream/" "origin/"; do
        local branch="${remote}${branch_name}"
        log "info" "Trying to find $branch_name at $branch"

        if git rev-parse --verify --quiet "$branch" >/dev/null 2>&1; then
            log "info" "Found $branch_name at \`$branch\`"
            git rev-parse "$branch"
            return 0
        else
            log "info" "Couldn't find $branch_name at \`$branch\`"
        fi
    done

    log "fatal" "Couldn't find $branch_name branch!"
}

# Check if commit1 is ancestor of commit2
is_ancestor() {
    local ancestor="$1"
    local descendant="$2"
    git merge-base --is-ancestor "$ancestor" "$descendant" 2>/dev/null
}

# Ensure the named branch is related to (at, an ancestor of, or descended from) our commit.
#
# Param: branch_name - A branch name. As a convenience, if not fully qualified, and no such
#                      branch exists locally, we will try to find it at origin/ and upstream/.
# Returns: The branch name.
# Error: If the named branch is not related to our commit.
validate_branch() {
    local branch_name="$1"

    local branch_commit
    branch_commit=$(find_branch "$branch_name")

    if [[ "$branch_commit" == "$COMMIT" ]]; then
        log "debug" "Branch matches commit"
        echo "$branch_name"
        return 0
    fi

    if is_ancestor "$COMMIT" "$branch_commit"; then
        log "debug" "Commit is an ancestor of branch: you are requesting a version for an old commit!"
        echo "$branch_name"
        return 0
    fi

    if is_ancestor "$branch_commit" "$COMMIT"; then
        log "debug" "Branch is an ancestor of commit: you are requesting a version for an unmerged commit!"
        echo "$branch_name"
        return 0
    fi

    # Die
    local short_commit short_branch
    short_commit=$(git rev-parse --short=8 "$COMMIT")
    short_branch=$(git rev-parse --short=8 "$branch_commit")
    log "fatal" "Commit $short_commit and branch $branch_name ($short_branch) do not appear to be related!"
}

branch_from_commit() {
    local commit_ish="${1:-}"

    local -a here=()
    local -a ancestors=()
    local -a descendants=()

    # If we got an explicit commit_ish, and it's already a branch name, use it.
    if [[ -n "$commit_ish" ]]; then
        # Exact match
        if git show-ref --verify --quiet "refs/heads/$commit_ish" 2>/dev/null; then
            log "debug" "Found a branch for commit_ish $commit_ish"
            echo "$commit_ish"
            return 0
        fi

        # Match minus remote name
        while IFS= read -r ref; do
            local remote_head
            if [[ "$ref" =~ refs/remotes/([^/]+)/(.+)$ ]]; then
                local remote_name="${BASH_REMATCH[1]}"
                remote_head="${BASH_REMATCH[2]}"

                if [[ "$remote_head" == "$commit_ish" ]]; then
                    log "debug" "Found a branch for commit_ish $commit_ish at remote '$remote_name'"
                    echo "$commit_ish"
                    return 0
                fi
            fi
        done < <(git for-each-ref --format='%(refname)' refs/remotes/ 2>/dev/null)
    fi

    log "debug" "Trying to discover a suitable branch from commit $(shortsha)..."

    while IFS= read -r ref; do
        local refname ref_commit

        # Extract reference name
        if [[ "$ref" =~ ^refs/remotes/[^/]+/(.+)$ ]]; then
            refname="${BASH_REMATCH[1]}"
        elif [[ "$ref" =~ ^refs/heads/(.+)$ ]]; then
            refname="${BASH_REMATCH[1]}"
        else
            continue
        fi

        # Skip HEAD and konflux branches
        [[ "$refname" == "HEAD" ]] && continue
        [[ "$refname" =~ konflux/ ]] && continue

        ref_commit=$(git rev-parse "$ref" 2>/dev/null)

        # Okay, now start using $COMMIT, which is already rev_parse()d.
        # If this ref is at our commit, it's a candidate
        if [[ "$ref_commit" == "$COMMIT" ]]; then
            here+=("$refname")
        elif is_ancestor "$ref_commit" "$COMMIT"; then
            ancestors+=("$refname")
        elif is_ancestor "$COMMIT" "$ref_commit"; then
            descendants+=("$refname")
        fi
    done < <(git for-each-ref --format='%(refname)' refs/heads/ refs/remotes/ 2>/dev/null)

    # First look for a unique branch right here
    if [[ ${#here[@]} -eq 1 ]]; then
        log "debug" "Found unique branch ${here[0]} at commit $(shortsha)"
        echo "${here[0]}"
        return 0
    fi

    if [[ ${#here[@]} -gt 1 ]]; then
        # We can't handle this
        log "fatal" "Found ${#here[@]} branches at commit $(shortsha)! Please specify a branch name explicitly."
    fi

    # Next look for named descendants of our commit. This indicates we're versioning an old commit on the branch.
    if [[ ${#descendants[@]} -eq 1 ]]; then
        log "debug" "Found branch ${descendants[0]} descended from commit $(shortsha)"
        echo "${descendants[0]}"
        return 0
    fi

    log "debug" "descendants: ${descendants[*]}"

    if [[ ${#descendants[@]} -eq 0 ]]; then
        log "warning" "WARNING: Are you versioning an unmerged commit?"
    fi

    if [[ ${#ancestors[@]} -eq 1 ]]; then
        log "debug" "Found branch ${ancestors[0]} which is an ancestor of commit $(shortsha)"
        echo "${ancestors[0]}"
        return 0
    fi

    log "fatal" "Could not determine a branch! Please specify one explicitly."
}

# Version initialization
version_init() {
    local branch_name_arg="${1:-}"
    local commit_ish_arg="${2:-}"

    # Determine commit
    if [[ -n "$commit_ish_arg" ]]; then
        # Commit explicitly given
        COMMIT=$(git rev-parse "$commit_ish_arg")
    else
        # Assume HEAD. If we got branch_name but not commit_ish, and they're sitting somewhere unrelated,
        # that's a user error. (To build on the branch commit, just specify the branch to commit_ish.)
        COMMIT=$(git rev-parse HEAD)
    fi

    # Determine branch name
    if [[ -n "$branch_name_arg" ]]; then
        BRANCH_NAME=$(validate_branch "$branch_name_arg")
    else
        # Before looking for related refs:
        # If we have an active branch, and it's equal to our commit, use it.
        if git symbolic-ref -q HEAD >/dev/null 2>&1; then
            local active_branch
            active_branch=$(git rev-parse --abbrev-ref HEAD)

            if [[ "$(git rev-parse HEAD)" == "$COMMIT" ]]; then
                BRANCH_NAME="$active_branch"
                log "debug" "Using active branch $BRANCH_NAME since it corresponds to commit $(shortsha)"
            fi
        else
            # Detached HEAD: In CI builds (Konflux), always assume master since all
            # Konflux pipelines target master branch, even if other branch refs exist.
            log "debug" "No active branch (detached HEAD), assuming master for CI build"
            BRANCH_NAME="master"
        fi
    fi

    # Calculate prefix and commit count
    PREFIX=$(prefix_from_branch "$BRANCH_NAME")
    local parent
    parent=$(git rev-list --max-parents=0 "$COMMIT")
    COMMIT_COUNT=$(git rev-list --count "${parent}..${COMMIT}")
}

# Get semver string
semver() {
    echo "${PREFIX}.${COMMIT_COUNT}-$(shortsha)"
}

# String representation
version_string() {
    echo "v$(semver)"
}

# Main
main() {
    MODE="standalone"

    # Get repo directory (parent of hack/)
    local repo_dir
    repo_dir=$(dirname "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")")

    # Change to repo directory
    cd "$repo_dir"

    # Initialize version
    version_init

    # Print version string
    version_string
}

# Run main if executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main
fi
