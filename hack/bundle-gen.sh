#!/bin/bash

set -euo pipefail

SCRIPT_DIR=$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")
# shellcheck source=hack/version2.sh
source "$SCRIPT_DIR/version2.sh"

# Constants
readonly HIVE_REPO_DEFAULT="git@github.com:openshift/hive.git"
# Hive dir within both:
# https://github.com/redhat-openshift-ecosystem/community-operators-prod
# https://github.com/k8s-operatorhub/community-operators
readonly HIVE_SUB_DIR="operators/hive-operator"
readonly IMAGE_REPO_DEFAULT="quay.io/openshift-hive/hive"
readonly COMMUNITY_OPERATORS_UPSTREAM="${COMMUNITY_OPERATORS_UPSTREAM:-git@github.com:redhat-openshift-ecosystem/community-operators-prod.git}"
readonly CHANNEL_DEFAULT="alpha"
readonly YQ="${YQ:-yq}"

# Runtime state — set by parse_args
VERBOSE=false
DRY_RUN=false
HOLD=false
SKIP_RELEASE_CONFIG=false
SKIP_IMAGE_VALIDATION=false
GITHUB_USER="${GITHUB_USER:-${USER}}"
HIVE_REPO="$HIVE_REPO_DEFAULT"
IMAGE_REPO="$IMAGE_REPO_DEFAULT"
IMAGE_TAG_OVERRIDE=""
COMMIT_ISH=""
DUMMY_BUNDLE=false

# Temp dirs — populated in main, removed by cleanup trap
HIVE_REPO_DIR=""
BUNDLE_DIR=""
WORK_DIR=""

# usage
usage() {
    cat <<EOF
Usage: $(basename "$0") [OPTIONS]

Hive Bundle Generator and Publishing Script.

This utility will:
  1. Clone the repo specified by --hive-repo (default: $HIVE_REPO_DEFAULT).
  2. Check out the commit-ish specified by --commit (default: tip of master).
  3. Look for a hive image in --image-repo with the tag specified by
     --image-tag-override (default: 10-char SHA of the checked-out commit).
     - If not found, build and push that image.
     - Skip this step with --skip-image-validation or if the image
       repo is not quay.io (non-quay images are always treated as present).
  4. Generate an OperatorHub bundle from the checked-out commit, pointing to
     the image found or built above.
  5. (Without --dummy-bundle) Open PRs with this bundle in the Red Hat and
     upstream Kubernetes community operator projects:
       github.com/redhat-openshift-ecosystem/community-operators-prod
       github.com/k8s-operatorhub/community-operators

OPTIONS:
  --verbose                   Show more details while running
  --dry-run                   Skip building/pushing images, pushing branches,
                              and submitting PRs
  --github-user USER          GitHub username (default: \$GITHUB_USER then \$USER)
  --hold                      Add /hold in PR body to prevent auto-merge
                              (use "/hold cancel" to remove)
  --hive-repo REPO            Hive git repo to clone. Save time by pointing at
                              a local directory instead of the remote, but make
                              sure it is up to date. (default: $HIVE_REPO_DEFAULT)
  --skip-release-config       Skip generating release-config.yaml, which is
                              required for bundles to be deployed on OperatorHubs.
                              (OperatorHub mode only; no effect with --dummy-bundle)
  --image-repo REPO           Image repository, e.g. quay.io/myproject/hive.
                              Do not include a tag; it will be generated based
                              on the branch/commit. (default: $IMAGE_REPO_DEFAULT)
  --image-tag-override TAG    Override the computed image tag. By default the
                              first 10 digits of the SHA of the commit are used.
  --commit COMMIT-ISH         Commit-ish to build from (default: tip of master).
                              Accepts any ref resolvable by git rev-parse. In
                              OperatorHub mode, this should be a commit on master.
  --dummy-bundle              Generate bundle files locally only — no PRs, no
                              release-config, no version graph directives
                              (replaces, skipRange, etc.). Version is computed
                              as 1.2.\$count-\$sha where \$count is the number of
                              commits to that point and \$sha is the first 7
                              digits of the commit SHA. Bundle is written to
                              ./hive-operator-bundle-vVER.
  --skip-image-validation     Skip checking/building/pushing the image. Note
                              that only quay.io images are validated; non-quay
                              images are always treated as present.
  -h, --help                  Show this help
EOF
}

# require_arg — guard for value-taking flags
# Exits if the next token is missing or looks like another flag.
require_arg() {
    local flag="$1" value="${2:-}"
    if [[ -z "$value" || "$value" == --* ]]; then
        echo "Error: $flag requires a non-empty argument" >&2
        usage >&2
        exit 1
    fi
}

# parse_args
parse_args() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --verbose)               VERBOSE=true ;;
            --dry-run)               DRY_RUN=true ;;
            --hold)                  HOLD=true ;;
            --skip-release-config)   SKIP_RELEASE_CONFIG=true ;;
            --skip-image-validation) SKIP_IMAGE_VALIDATION=true ;;
            # TODO: Validate this early — if wrong you won't bounce until open_pr.
            --github-user)           require_arg "$1" "${2:-}"; GITHUB_USER="$2";        shift ;;
            --hive-repo)             require_arg "$1" "${2:-}"; HIVE_REPO="$2";          shift ;;
            --image-repo)            require_arg "$1" "${2:-}"; IMAGE_REPO="$2";         shift ;;
            --image-tag-override)    require_arg "$1" "${2:-}"; IMAGE_TAG_OVERRIDE="$2"; shift ;;
            --commit)                require_arg "$1" "${2:-}"; COMMIT_ISH="$2";         shift ;;
            --dummy-bundle)          DUMMY_BUNDLE=true ;;
            -h|--help)               usage; exit 0 ;;
            *) echo "Unknown option: $1" >&2; usage >&2; exit 1 ;;
        esac
        shift
    done
}

# cleanup — registered as EXIT trap
cleanup() {
    [[ -n "$HIVE_REPO_DIR" ]] && rm -rf "$HIVE_REPO_DIR"
    [[ -n "$BUNDLE_DIR"    ]] && rm -rf "$BUNDLE_DIR"
    [[ -n "$WORK_DIR"      ]] && rm -rf "$WORK_DIR"
}

# validate_image
# Returns 0 if the image exists or validation is skipped; 1 if not found.
validate_image() {
    local image_repo="$1" image_tag="$2"

    if $SKIP_IMAGE_VALIDATION; then
        echo "Skipping image validation for ${image_repo}:${image_tag}"
        return 0
    fi

    # Only validate quay.io images
    local host="${image_repo%%/*}"
    if [[ "$host" != "quay.io" ]]; then
        echo "Skipping validation for non-quay image: ${image_repo}:${image_tag}"
        return 0
    fi

    local path="${image_repo#*/}"
    local url="https://quay.io/api/v1/repository/${path}/tag/?specificTag=${image_tag}"

    local resp
    if ! resp=$(curl -sf --max-time 30 "$url"); then
        echo "Failed to query quay API: $url" >&2
        exit 1
    fi

    if [[ "$(echo "$resp" | jq '.tags | length')" -eq 0 ]]; then
        echo "No image found at ${image_repo}:${image_tag}"
        return 1
    fi

    echo "Image validated: ${image_repo}:${image_tag}"
}

# ensure_image — validates; builds and pushes if missing.
# Must be called from the hive repo root (make target lives there).
ensure_image() {
    local image_repo="$1" image_tag="$2"
    local uri="${image_repo}:${image_tag}"

    validate_image "$image_repo" "$image_tag" && return 0

    if $DRY_RUN; then
        echo "DRY RUN: skipping build/push of $uri"
        return 0
    fi

    echo "Building image $uri"
    make "IMG=$uri" podman-operatorhub-build || { echo "Image build failed" >&2; exit 1; }

    echo "Pushing image $uri"
    podman push "$uri" || { echo "Image push failed" >&2; exit 1; }
}

# semver_gt — returns 0 if $1 > $2 using version-aware sort.
# Strips the git-hash suffix (everything after the first '-') before comparing
# so that e.g. "1.2.3200-abc1234" > "1.2.3187-18827f6".
semver_gt() {
    local a="${1%%-*}" b="${2%%-*}"
    [[ "$a" != "$b" ]] && \
        [[ "$(printf '%s\n%s' "$a" "$b" | sort -V | tail -1)" == "$a" ]]
}

# get_previous_version
# Clones community-operators-prod (reuses clone if present) and returns the
# highest version present in the given channel on stdout; all other output
# goes to stderr.
get_previous_version() {
    local channel="$1"
    local repo_path="$WORK_DIR/community-operators-prod"

    if [[ ! -d "$repo_path" ]]; then
        echo "Cloning $COMMUNITY_OPERATORS_UPSTREAM" >&2
        git clone "$COMMUNITY_OPERATORS_UPSTREAM" "$repo_path" >&2
    fi

    git -C "$repo_path" checkout main >&2

    local hive_dir="$repo_path/$HIVE_SUB_DIR"
    local highest="0.0.0"

    while IFS= read -r version_dir; do
        local version
        version=$(basename "$version_dir")

        local annotation="$version_dir/metadata/annotations.yaml"
        [[ -f "$annotation" ]] || continue

        local channels
        channels=$("$YQ" '.annotations["operators.operatorframework.io.bundle.channels.v1"] // ""' "$annotation")

        if [[ ",$channels," == *",$channel,"* ]]; then
            if semver_gt "$version" "$highest"; then
                highest="$version"
            fi
        fi
    done < <(find "$hive_dir" -maxdepth 1 -mindepth 1 -type d | sort)

    if [[ "$highest" == "0.0.0" ]]; then
        # NOTE: If a new channel is introduced, finding a prev_version will fail
        # and require a manual edit of the CSV with the prev_version. The exit is
        # intentionally fatal to ensure this failure is noticed rather than silently
        # producing a broken bundle.
        echo "Channel '$channel' not found in $COMMUNITY_OPERATORS_UPSTREAM" >&2
        exit 1
    fi

    echo "$highest"
}

# generate_bundle
# Creates the full bundle directory structure under $bundle_dir/$semver_ver:
#   metadata/annotations.yaml
#   manifests/<crds>
#   manifests/hive-operator.v<semver>.clusterserviceversion.yaml
#
# Must be called from the hive repo root (reads config/ relative paths).
generate_bundle() {
    local bundle_dir="$1" image_repo="$2" semver_ver="$3" image_tag="$4"

    local crds_dir="config/crds"
    local csv_template="config/templates/hive-csv-template.yaml"
    local operator_role="config/operator/operator_role.yaml"
    local deployment_spec="config/operator/operator_deployment.yaml"

    local version_dir="$bundle_dir/$semver_ver"
    local manifests_dir="$version_dir/manifests"
    local metadata_dir="$version_dir/metadata"
    mkdir -p "$manifests_dir" "$metadata_dir"

    echo "Writing bundle to: $version_dir"
    echo "Generating CSV for version: $semver_ver"

    # --- metadata/annotations.yaml ---
    cat > "$metadata_dir/annotations.yaml" <<EOF
annotations:
  operators.operatorframework.io.bundle.channel.default.v1: ${CHANNEL_DEFAULT}
  operators.operatorframework.io.bundle.channels.v1: ${CHANNEL_DEFAULT}
  operators.operatorframework.io.bundle.manifests.v1: manifests/
  operators.operatorframework.io.bundle.mediatype.v1: registry+v1
  operators.operatorframework.io.bundle.metadata.v1: metadata/
  operators.operatorframework.io.bundle.package.v1: hive-operator
EOF

    # --- Copy CRDs and build the owned-CRDs list for the CSV ---
    local owned_crds_file="$WORK_DIR/owned-crds.yaml"
    echo "[]" > "$owned_crds_file"

    local crd_file
    while IFS= read -r crd_file; do
        cp "$crd_file" "$manifests_dir/"

        local kind version_name crd_name description
        kind=$("$YQ" '.spec.names.kind' "$crd_file")
        version_name=$("$YQ" '.spec.versions[0].name' "$crd_file")
        crd_name=$("$YQ" '.metadata.name' "$crd_file")
        description=$("$YQ" '.spec.versions[0].schema.openAPIV3Schema.description // ""' "$crd_file")

        KIND="$kind" VERSION="$version_name" CRD_NAME="$crd_name" DESCRIPTION="$description" \
            "$YQ" -i '. += [{"description": env(DESCRIPTION), "displayName": env(KIND), "kind": env(KIND), "name": env(CRD_NAME), "version": env(VERSION)}]' \
            "$owned_crds_file"
    done < <(find "$crds_dir" -maxdepth 1 -type f \( -name '*.yaml' -o -name '*.yml' \) | sort)

    # --- Build CSV from template ---
    local csv_file="$manifests_dir/hive-operator.v${semver_ver}.clusterserviceversion.yaml"
    cp "$csv_template" "$csv_file"

    # Extract operator role rules and deployment spec into temp files so yq
    # can load them as structured YAML (avoids quoting/escaping issues).
    local rules_file="$WORK_DIR/rules.yaml"
    local deploy_spec_file="$WORK_DIR/deploy-spec.yaml"

    "$YQ" '.rules' "$operator_role" > "$rules_file"
    # operator_deployment.yaml is multi-document; index 1 is the Deployment.
    "$YQ" 'select(document_index == 1) | .spec' "$deployment_spec" > "$deploy_spec_file"

    local image_ref="${image_repo}:${image_tag}"
    local created_at
    created_at=$(date -u '+%Y-%m-%dT%H:%M:%SZ')

    "$YQ" -i "
        .metadata.name = \"hive-operator.v${semver_ver}\" |
        .spec.version = \"${semver_ver}\" |
        .spec.customresourcedefinitions.owned = load(\"${owned_crds_file}\") |
        .spec.install.spec.clusterPermissions = [{\"rules\": load(\"${rules_file}\"), \"serviceAccountName\": \"hive-operator\"}] |
        .spec.install.spec.deployments[0].spec = load(\"${deploy_spec_file}\") |
        .spec.install.spec.deployments[0].spec.template.spec.containers[0].image = \"${image_ref}\" |
        .metadata.annotations.containerImage = \"${image_ref}\" |
        .metadata.annotations.createdAt = \"${created_at}\"
    " "$csv_file"

    echo "Wrote ClusterServiceVersion: $csv_file"
}

# add_operatorhub_extras
# Adds OperatorHub-specific content to an already-generated bundle:
#   - sets spec.replaces in the CSV
#   - generates release-config.yaml (unless --skip-release-config)
add_operatorhub_extras() {
    local version_dir="$1" semver_ver="$2" prev_version="$3"

    local csv_file="$version_dir/manifests/hive-operator.v${semver_ver}.clusterserviceversion.yaml"
    "$YQ" -i ".spec.replaces = \"hive-operator.v${prev_version}\"" "$csv_file"

    if ! $SKIP_RELEASE_CONFIG; then
        # release-config.yaml is only used by the Red Hat Ecosystem for automatic
        # catalog updates across all RH catalogs. The Kubernetes Ecosystem ignores it.
        cat > "$version_dir/release-config.yaml" <<EOF
catalog_templates:
- channels:
  - ${CHANNEL_DEFAULT}
  replaces: hive-operator.v${prev_version}
  template_name: basic.yaml
EOF
    fi
}

# open_pr
# Pushes a branch to the user's fork and opens a PR upstream via gh CLI.
# Reuses an existing local clone when present (community-operators-prod may
# already be cloned by get_previous_version).
open_pr() {
    local fork_repo="$1" upstream_repo="$2" semver_ver="$3" bundle_dir="$4"

    local repo_name
    repo_name=$(basename "$fork_repo" .git)
    local repo_path="$WORK_DIR/$repo_name"

    # "git@github.com:org/repo.git" -> "org/repo"
    local gh_target
    gh_target=$(echo "$upstream_repo" | sed 's|git@github.com:||; s|\.git$||')

    if [[ ! -d "$repo_path" ]]; then
        echo "Cloning $fork_repo"
        git clone "$fork_repo" "$repo_path"
    else
        echo "Reusing existing clone at $repo_path"
    fi

    git -C "$repo_path" remote set-url origin "$fork_repo"
    git -C "$repo_path" remote add upstream "$upstream_repo" 2>/dev/null || \
        git -C "$repo_path" remote set-url upstream "$upstream_repo"

    echo "Fetching upstream $upstream_repo"
    git -C "$repo_path" fetch upstream

    git -C "$repo_path" switch --detach upstream/main

    local branch_name="update-hive-${semver_ver}"
    echo "Creating branch $branch_name"
    git -C "$repo_path" switch -C "$branch_name"

    echo "Copying bundle"
    local bundle_dest="$repo_path/$HIVE_SUB_DIR/$semver_ver"
    if [[ -e "$bundle_dest" ]]; then
        echo "Error: bundle version $semver_ver already exists in $gh_target" >&2
        exit 1
    fi
    cp -r "$bundle_dir/$semver_ver" "$bundle_dest"

    local pr_title="operator hive-operator (${semver_ver})"
    git -C "$repo_path" add "$HIVE_SUB_DIR"
    git -C "$repo_path" commit --signoff --message="$pr_title"

    if $DRY_RUN; then
        echo "DRY RUN: skipping push and PR for $gh_target"
        return 0
    fi

    echo "Pushing branch $branch_name to origin"
    git -C "$repo_path" push origin "$branch_name" --force

    local body="$pr_title"
    $HOLD && body="${body}"$'\n\n/hold'

    echo "Opening PR to $gh_target"
    gh pr create \
        --repo "$gh_target" \
        --head "${GITHUB_USER}:${branch_name}" \
        --base main \
        --title "$pr_title" \
        --body "$body"
}

# check_deps — fail fast with a clear message if required tools are missing
check_deps() {
    local required=("$YQ" git)
    # jq is only needed to query the quay.io API; skipped for non-quay repos
    # or when --skip-image-validation is set
    if ! $SKIP_IMAGE_VALIDATION && [[ "${IMAGE_REPO%%/*}" == "quay.io" ]]; then
        required+=(jq)
    fi
    # gh is only needed to open PRs (not in dummy-bundle or dry-run mode)
    if ! $DUMMY_BUNDLE && ! $DRY_RUN; then
        required+=(gh)
    fi

    local missing=()
    for cmd in "${required[@]}"; do
        command -v "$cmd" &>/dev/null || missing+=("$cmd")
    done
    if [[ ${#missing[@]} -gt 0 ]]; then
        echo "Error: missing required tool(s): ${missing[*]}" >&2
        echo "  yq (v4): https://github.com/mikefarah/yq" >&2
        echo "  gh:      https://cli.github.com/" >&2
        exit 1
    fi

    local yq_version
    yq_version=$("$YQ" --version 2>&1)
    if ! grep -q 'version v4' <<<"$yq_version"; then
        echo "Error: yq v4 is required (found: $yq_version)" >&2
        echo "  yq (v4): https://github.com/mikefarah/yq" >&2
        exit 1
    fi
}

# main
main() {
    parse_args "$@"
    check_deps
    $VERBOSE && set -x

    trap cleanup EXIT

    local orig_wd="$PWD"

    HIVE_REPO_DIR=$(mktemp -d --tmpdir hive-repo-XXXXXX)
    BUNDLE_DIR=$(mktemp -d --tmpdir hive-operator-bundle-XXXXXX)
    WORK_DIR=$(mktemp -d --tmpdir operatorhub-push-XXXXXX)

    echo "Cloning $HIVE_REPO to $HIVE_REPO_DIR"
    git clone "$HIVE_REPO" "$HIVE_REPO_DIR"

    cd "$HIVE_REPO_DIR"

    # version2.sh sets globals: COMMIT, PREFIX, COMMIT_COUNT
    # PREFIX is always 1.2 (master branch prefix) regardless of local branch.
    version_init "$COMMIT_ISH"

    local ver_semver image_tag
    ver_semver=$(semver)
    image_tag="${IMAGE_TAG_OVERRIDE:-${COMMIT:0:10}}"

    echo "Checking out $(shortsha)"
    git checkout "$COMMIT"

    ensure_image "$IMAGE_REPO" "$image_tag"

    if $DUMMY_BUNDLE; then
        # Mode 1: local bundle only — no version graph, no release-config, no PRs
        generate_bundle "$BUNDLE_DIR" "$IMAGE_REPO" "$ver_semver" "$image_tag"
        local dest="$orig_wd/hive-operator-bundle-v${ver_semver}"
        # Remove any previous output for this version so cp -r replaces it
        # cleanly rather than nesting the new bundle inside the existing dir.
        rm -rf "$dest"
        cp -r "$BUNDLE_DIR/$ver_semver" "$dest"
        echo "Wrote bundle to $dest"
    else
        # Mode 2: OperatorHub push — bundle + graph directives + PRs

        local prev_version
        prev_version=$(get_previous_version "$CHANNEL_DEFAULT")

        if [[ "$ver_semver" == "$prev_version" ]]; then
            echo "Error: version $ver_semver already exists upstream" >&2
            exit 1
        fi
        if ! semver_gt "$ver_semver" "$prev_version"; then
            echo "Error: version $ver_semver is lower than the current upstream version $prev_version" >&2
            echo "  A newer bundle has already been published. Target a more recent commit on master." >&2
            exit 1
        fi

        generate_bundle "$BUNDLE_DIR" "$IMAGE_REPO" "$ver_semver" "$image_tag"
        add_operatorhub_extras "$BUNDLE_DIR/$ver_semver" "$ver_semver" "$prev_version"

        open_pr \
            "git@github.com:${GITHUB_USER}/community-operators-prod.git" \
            "git@github.com:redhat-openshift-ecosystem/community-operators-prod.git" \
            "$ver_semver" "$BUNDLE_DIR"

        open_pr \
            "git@github.com:${GITHUB_USER}/community-operators.git" \
            "git@github.com:k8s-operatorhub/community-operators.git" \
            "$ver_semver" "$BUNDLE_DIR"
    fi
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi
