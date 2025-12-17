#!/bin/bash

set -e

# Local e2e test script - easier to run e2e tests locally
# This script provides sensible defaults and can be customized via environment variables
#
# Usage:
#   # Simplest: just run it (will auto-detect ~/.kube/config and check connectivity)
#   ./hack/local-e2e-test.sh
#   # or
#   make test-e2e-local
#
#   # Custom KUBECONFIG:
#   export KUBECONFIG=/path/to/kubeconfig
#   ./hack/local-e2e-test.sh
#
#   # Custom images (optional):
#   export HIVE_IMAGE="your-hive-image"
#   export RELEASE_IMAGE="your-release-image"
#   ./hack/local-e2e-test.sh

# Source common setup functions
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
source "${DIR}/local-e2e-common.sh"

# Set custom title for configuration output
export CONFIG_TITLE="Running Hive e2e test locally"

# Run setup - this sets up and exports all environment variables
setup_cluster_profile_dir
setup_shared_dir
setup_kubeconfig
check_cluster_connectivity
setup_hive_image
setup_release_image
select_cloud_platform
setup_cloud_credentials
setup_base_domain
setup_common_variables

# Print configuration
print_config

# Execute e2e test
${DIR}/e2e-test.sh
