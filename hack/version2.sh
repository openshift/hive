#!/bin/bash

set -euo pipefail

# Global state
# Version prefix for images and bundles; always uses the master branch prefix.
# Full version format: "v{PREFIX}.{commitcount}-{sha}" e.g. "v1.2.3187-18827f6"
PREFIX="1.2"
COMMIT=""
COMMIT_COUNT=""

# Get short SHA (7 characters)
shortsha() {
    git rev-parse --short=7 "$COMMIT" 2>/dev/null
}

# version_init [COMMIT-ISH]
# Resolves and sets the COMMIT and COMMIT_COUNT globals.
# COMMIT-ISH is any ref resolvable by git rev-parse (default: HEAD).
version_init() {
    local commit_ish_arg="${1:-}"

    # Determine commit
    if [[ -n "$commit_ish_arg" ]]; then
        # ^{commit} peels annotated tags to the underlying commit SHA so
        # COMMIT is always a commit object, keeping shortsha() correct.
        if ! COMMIT=$(git rev-parse --verify "${commit_ish_arg}^{commit}" 2>/dev/null); then
            echo "Error: '$commit_ish_arg' could not be resolved. Ensure the commit exists and the repository is up to date." >&2
            exit 1
        fi
    else
        # Assume HEAD
        COMMIT=$(git rev-parse HEAD)
    fi

    # Calculate commit count
    local parent
    parent=$(git rev-list --max-parents=0 "$COMMIT")
    COMMIT_COUNT=$(git rev-list --count "${parent}..${COMMIT}")
}

# Get semver string
semver() {
    echo "${PREFIX}.${COMMIT_COUNT}-$(shortsha)"
}

# Returns the full version string with a leading 'v' (e.g. v1.2.3187-18827f6)
version_string() {
    echo "v$(semver)"
}

# Main
main() {
    if [[ $# -gt 1 ]]; then
        echo "Usage: $(basename "$0") [COMMIT-ISH]" >&2
        exit 1
    fi

    # Get repo directory (parent of hack/)
    local repo_dir
    repo_dir=$(dirname "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")")

    # Change to repo directory
    cd "$repo_dir"

    # Initialize version
    version_init "${1:-}"

    # Print version string
    version_string
}

# Run main if executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi
