#!/bin/bash
declare -r UNKNOWN_BRANCH_PREFIX="0.0"
declare -r MASTER_BRANCH_PREFIX="1.2"

function prefix_from_branch() {
	declare -r branch_name="$1"

	if [[ "$branch_name" =~ ^([^/]+/)?mce-([[:digit:]]+.[[:digit:]]+) ]]; then
		echo "${BASH_REMATCH[2]}"
	elif [[ "$branch_name" =~ ^([^/]+/)?master$ ]]; then
		echo "$MASTER_BRANCH_PREFIX"
	else
		echo "$UNKNOWN_BRANCH_PREFIX"
	fi
}

function commit_count() {
	git rev-list --count "$(git rev-list --max-parents=0 HEAD)..HEAD"
}

function short_sha() {
	git rev-parse --short HEAD
}

echo "$(prefix_from_branch "$(git rev-parse --abbrev-ref HEAD)").$(commit_count)-$(short_sha)"
