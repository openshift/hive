#!/bin/bash
# Common functions and setup for local e2e test scripts
# This script is sourced by local-e2e-test.sh and local-e2e-pool-test.sh

# Supported cloud platforms (keep in sync with Makefile test-e2e-local target)
SUPPORTED_CLOUDS="aws/azure/gcp/ibmcloud/vsphere/nutanix/openstack"

# Setup and validate KUBECONFIG
# If KUBECONFIG environment variable is set, use it; otherwise use default ~/.kube/config
function setup_kubeconfig() {
    if [ -n "$KUBECONFIG" ]; then
        echo "Using KUBECONFIG from environment: $KUBECONFIG"
        if [ ! -f "$KUBECONFIG" ]; then
            echo "Error: KUBECONFIG file not found: $KUBECONFIG" >&2
            exit 1
        fi
    else
        # Auto-detect: use default kubeconfig location if KUBECONFIG is not set
        DEFAULT_KUBECONFIG="${HOME}/.kube/config"
        if [ -f "$DEFAULT_KUBECONFIG" ]; then
            export KUBECONFIG="$DEFAULT_KUBECONFIG"
            echo "KUBECONFIG not set in environment, using default: $KUBECONFIG"
        else
            echo "Error: KUBECONFIG environment variable is not set and default kubeconfig not found at $DEFAULT_KUBECONFIG" >&2
            echo "Please set KUBECONFIG environment variable:" >&2
            echo "  export KUBECONFIG=/path/to/your/kubeconfig" >&2
            exit 1
        fi
    fi
}

# Check cluster connectivity using oc or kubectl
# Requires KUBECONFIG to be set (should be called after setup_kubeconfig)
function check_cluster_connectivity() {
    if [ -z "$KUBECONFIG" ]; then
        echo "Error: KUBECONFIG is not set. Please call setup_kubeconfig() first." >&2
        exit 1
    fi
    echo "Checking cluster connectivity with kubeconfig: $KUBECONFIG"
    CLUSTER_ACCESSIBLE=false
    if command -v oc &> /dev/null; then
        if KUBECONFIG="$KUBECONFIG" oc cluster-info &> /dev/null; then
            echo "✓ Cluster is accessible via oc"
            CLUSTER_ACCESSIBLE=true
        fi
    fi

    if [ "$CLUSTER_ACCESSIBLE" = "false" ] && command -v kubectl &> /dev/null; then
        if KUBECONFIG="$KUBECONFIG" kubectl cluster-info &> /dev/null; then
            echo "✓ Cluster is accessible via kubectl"
            CLUSTER_ACCESSIBLE=true
        fi
    fi

    if [ "$CLUSTER_ACCESSIBLE" = "false" ]; then
        if ! command -v oc &> /dev/null && ! command -v kubectl &> /dev/null; then
            echo "Error: Neither oc nor kubectl found. Please install one of them." >&2
        else
            echo "Error: Cannot connect to cluster using KUBECONFIG=$KUBECONFIG" >&2
            echo "Please verify:" >&2
            echo "  1. The kubeconfig file is valid" >&2
            echo "  2. The cluster is accessible" >&2
            echo "  3. Your credentials are valid" >&2
        fi
        exit 1
    fi
}

# Setup HIVE_IMAGE: auto-detect, prompt, or use existing value
# Must export the variable so e2e-common.sh and child processes can access it
function setup_hive_image() {
    if [ -z "$HIVE_IMAGE" ]; then
        echo "HIVE_IMAGE not set, attempting to get latest tag from quay.io..."
        if ! command -v curl &> /dev/null || ! command -v jq &> /dev/null; then
            echo "Warning: curl or jq not found. Cannot auto-detect Hive image tag." >&2
            echo "Please install curl and jq, or set HIVE_IMAGE environment variable." >&2
            echo "Please enter the Hive image to use (e.g. quay.io/my-user/hive:latest):"
            read -r HIVE_IMAGE
            if [ -z "$HIVE_IMAGE" ]; then
                echo "Error: HIVE_IMAGE cannot be empty" >&2
                exit 1
            fi
        else
            HIVE_IMAGE_REPO="${HIVE_IMAGE_REPO:-redhat-user-workloads/crt-redhat-acm-tenant/hive-operator/hive}"
            LATEST_TAG=$(curl -sk "https://quay.io/api/v1/repository/${HIVE_IMAGE_REPO}/tag/?page=1&limit=100" | \
                jq -r '.tags | map(select(.name | startswith("hive-on-push") and endswith("-index"))) | sort_by(.start_ts) | reverse | .[0].name // empty' 2>/dev/null)
            if [ -n "$LATEST_TAG" ] && [ "$LATEST_TAG" != "null" ] && [ "$LATEST_TAG" != "" ]; then
                HIVE_IMAGE="quay.io/${HIVE_IMAGE_REPO}:${LATEST_TAG}"
                echo "Auto-detected HIVE_IMAGE: $HIVE_IMAGE"
            else
                echo "Failed to auto-detect Hive image tag"
                echo "Please enter the Hive image to use (e.g. quay.io/my-user/hive:latest):"
                read -r HIVE_IMAGE
                if [ -z "$HIVE_IMAGE" ]; then
                    echo "Error: HIVE_IMAGE cannot be empty" >&2
                    exit 1
                fi
            fi
        fi
    fi
    export HIVE_IMAGE="$HIVE_IMAGE"
}

# Setup RELEASE_IMAGE: user input or auto-detect from cluster
# Must export the variable so e2e-common.sh and child processes can access it
function setup_release_image() {
    if [ -z "$RELEASE_IMAGE" ]; then
        echo "RELEASE_IMAGE not set"
        echo "Please choose:"
        echo "  1. Enter RELEASE_IMAGE manually"
        echo "  2. Auto-detect from cluster (requires KUBECONFIG and oc/kubectl)"
        echo ""
        echo "Enter choice [1 or 2, default: 2]:"
        read -r choice
        choice="${choice:-2}"
        
        if [ "$choice" = "1" ]; then
            echo "Please enter the RELEASE_IMAGE (e.g. registry.ci.openshift.org/ocp/release:4.21.0-0.nightly):"
            read -r RELEASE_IMAGE
            if [ -z "$RELEASE_IMAGE" ]; then
                echo "Error: RELEASE_IMAGE cannot be empty" >&2
                exit 1
            fi
            export RELEASE_IMAGE="$RELEASE_IMAGE"
            echo "Using RELEASE_IMAGE: $RELEASE_IMAGE"
        else
            # Auto-detect from cluster
            echo "Attempting to auto-detect RELEASE_IMAGE from cluster..."
            DETECTED_RELEASE_IMAGE=""
            if [ -n "$KUBECONFIG" ]; then
                if command -v oc &> /dev/null; then
                    DETECTED_RELEASE_IMAGE=$(KUBECONFIG="$KUBECONFIG" oc get clusterversion version -o=jsonpath='{.status.desired.image}' 2>/dev/null || true)
                elif command -v kubectl &> /dev/null; then
                    DETECTED_RELEASE_IMAGE=$(KUBECONFIG="$KUBECONFIG" kubectl get clusterversion version -o=jsonpath='{.status.desired.image}' 2>/dev/null || true)
                fi
            fi
            
            if [ -n "$DETECTED_RELEASE_IMAGE" ]; then
                export RELEASE_IMAGE="$DETECTED_RELEASE_IMAGE"
                echo "Auto-detected RELEASE_IMAGE from cluster: $RELEASE_IMAGE"
            else
                echo "Error: Could not detect RELEASE_IMAGE from cluster" >&2
                echo "Please ensure:" >&2
                echo "  1. KUBECONFIG is set and points to a valid cluster" >&2
                echo "  2. oc or kubectl is installed and accessible" >&2
                echo "  3. The cluster has a clusterversion resource" >&2
                echo "" >&2
                echo "You can set RELEASE_IMAGE manually by running the script again and choosing option 1" >&2
                exit 1
            fi
        fi
    else
        export RELEASE_IMAGE="$RELEASE_IMAGE"
    fi
}

# Select cloud platform interactively if not set
function select_cloud_platform() {
    if [ -z "$CLOUD" ]; then
        echo "CLOUD environment variable is not set"
        echo "Please enter the cloud platform (${SUPPORTED_CLOUDS}):"
        read -r CLOUD
        CLOUD="${CLOUD:-aws}"
    fi
    export CLOUD="$CLOUD"
}

# Setup CLUSTER_PROFILE_DIR early so e2e-common.sh can use it
function setup_cluster_profile_dir() {
    # CLUSTER_PROFILE_DIR is used by e2e-common.sh to find credential files
    # Set it early so e2e-common.sh will use our value instead of default /tmp/cluster
    CLUSTER_PROFILE_DIR="${CLUSTER_PROFILE_DIR:-${HOME}/tmp/hive-e2e}"
    export CLUSTER_PROFILE_DIR="$CLUSTER_PROFILE_DIR"
    mkdir -p "$CLUSTER_PROFILE_DIR"
}

# Setup SHARED_DIR for platforms that need it (e.g., OpenStack)
# SHARED_DIR is used by e2e-common.sh for OpenStack resources
# Requires CLUSTER_PROFILE_DIR to be set (should be called after setup_cluster_profile_dir)
function setup_shared_dir() {
    if [ -z "$CLUSTER_PROFILE_DIR" ]; then
        echo "Error: CLUSTER_PROFILE_DIR is not set. Please call setup_cluster_profile_dir() first." >&2
        exit 1
    fi
    # If SHARED_DIR is not set, use CLUSTER_PROFILE_DIR so all files are in one place
    if [ -z "$SHARED_DIR" ]; then
        export SHARED_DIR="$CLUSTER_PROFILE_DIR"
    fi
}

# Setup cloud credentials based on selected platform
# Creates symlinks from CLUSTER_PROFILE_DIR to local credential files
# Requires CLOUD and CLUSTER_PROFILE_DIR to be set (should be called after select_cloud_platform and setup_cluster_profile_dir)
function setup_cloud_credentials() {
    if [ -z "$CLOUD" ]; then
        echo "Error: CLOUD is not set. Please call select_cloud_platform() first." >&2
        exit 1
    fi
    if [ -z "$CLUSTER_PROFILE_DIR" ]; then
        echo "Error: CLUSTER_PROFILE_DIR is not set. Please call setup_cluster_profile_dir() first." >&2
        exit 1
    fi
    # Variable to store credential file path for output (global scope for print_config)
    CREDS_FILE=""

    case "$CLOUD" in
    "aws")
        # AWS: Can use environment variables or credential file
        # e2e-common.sh checks: if [ -s $CREDS_FILE ] && ! [ "${AWS_ACCESS_KEY_ID}" ] && ! [ "${AWS_SECRET_ACCESS_KEY}" ]
        if [ -z "$AWS_ACCESS_KEY_ID" ] || [ -z "$AWS_SECRET_ACCESS_KEY" ]; then
            CREDS_FILE="${CLUSTER_PROFILE_DIR}/.awscred"
            if [ ! -s "$CREDS_FILE" ] && [ -s "${HOME}/.aws/credentials" ]; then
                # Create symlink to local AWS credentials
                ln -sf "${HOME}/.aws/credentials" "$CREDS_FILE"
                echo "Using AWS credentials from ~/.aws/credentials (linked to ${CREDS_FILE})"
            elif [ -s "$CREDS_FILE" ]; then
                echo "Using AWS credentials from ${CREDS_FILE}"
            else
                echo "Warning: AWS credentials not found. Please either:" >&2
                echo "  1. Set AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY environment variables" >&2
                echo "  2. Configure ~/.aws/credentials" >&2
                echo "  3. Create ${CREDS_FILE}" >&2
            fi
        else
            echo "Using AWS credentials from environment variables"
            CREDS_FILE="(environment variables)"
        fi
        ;;
    "azure")
        # Azure: e2e-common.sh expects ${CLUSTER_PROFILE_DIR}/osServicePrincipal.json
        CREDS_FILE="${CLUSTER_PROFILE_DIR}/osServicePrincipal.json"
        if [ ! -e "$CREDS_FILE" ] && [ -f "${HOME}/.azure/osServicePrincipal.json" ]; then
            # Create symlink to local Azure credentials
            ln -sf "${HOME}/.azure/osServicePrincipal.json" "$CREDS_FILE"
            echo "Using Azure credentials from ~/.azure/osServicePrincipal.json (linked to ${CREDS_FILE})"
        elif [ -e "$CREDS_FILE" ]; then
            echo "Using Azure credentials from ${CREDS_FILE}"
        else
            echo "Error: Azure credentials file not found" >&2
            echo "Expected at: ~/.azure/osServicePrincipal.json or ${CREDS_FILE}" >&2
            exit 1
        fi
        ;;
    "gcp")
        # GCP: e2e-common.sh expects ${CLUSTER_PROFILE_DIR}/gce.json
        CREDS_FILE="${CLUSTER_PROFILE_DIR}/gce.json"
        if [ ! -e "$CREDS_FILE" ] && [ -f "${HOME}/.gcp/osServiceAccount.json" ]; then
            # Create symlink to local GCP credentials
            ln -sf "${HOME}/.gcp/osServiceAccount.json" "$CREDS_FILE"
            echo "Using GCP credentials from ~/.gcp/osServiceAccount.json (linked to ${CREDS_FILE})"
        elif [ -e "$CREDS_FILE" ]; then
            echo "Using GCP credentials from ${CREDS_FILE}"
        else
            echo "Error: GCP credentials file not found" >&2
            echo "Expected at: ~/.gcp/osServiceAccount.json or ${CREDS_FILE}" >&2
            exit 1
        fi
        ;;
    "openstack")
        # OpenStack: e2e-common.sh expects ${SHARED_DIR}/clouds.yaml
        # Hive reads the default path ~/.config/openstack/clouds.yaml as credentials
        # SHARED_DIR should be set in setup_shared_dir() if not already set
        #
        # Required files in ${SHARED_DIR} for OpenStack:
        #   - clouds.yaml: OpenStack credentials (auto-linked from ~/.config/openstack/clouds.yaml if exists)
        #   - HIVE_FIP_API: API floating IP address
        #   - HIVE_FIP_INGRESS: Ingress floating IP address
        #   - OPENSTACK_EXTERNAL_NETWORK: External network name
        #   - OPENSTACK_COMPUTE_FLAVOR: Compute flavor name
        #   - OPENSTACK_CONTROLPLANE_FLAVOR: Control plane flavor name
        #   - HIVE_CLUSTER_NAME: Cluster name (used by e2e-test.sh)
       
        # These files are read by e2e-common.sh via get_osp_resources function
        # It should be prepared in ${SHARED_DIR} before running the test
        # If files don't exist, e2e-common.sh will exit with an error
        if [ -z "$SHARED_DIR" ]; then
            echo "Error: SHARED_DIR is not set. Please call setup_shared_dir() first." >&2
            exit 1
        fi
        CREDS_FILE="${SHARED_DIR}/clouds.yaml"
        if [ ! -e "$CREDS_FILE" ] && [ -f "${HOME}/.config/openstack/clouds.yaml" ]; then
            # Create symlink to local OpenStack credentials
            ln -sf "${HOME}/.config/openstack/clouds.yaml" "$CREDS_FILE"
            echo "Using OpenStack credentials from ~/.config/openstack/clouds.yaml (linked to ${CREDS_FILE})"
        elif [ -e "$CREDS_FILE" ]; then
            echo "Using OpenStack credentials from ${CREDS_FILE}"
        else
            echo "Error: OpenStack credentials file not found" >&2
            echo "Expected at: ~/.config/openstack/clouds.yaml or ${CREDS_FILE}" >&2
            exit 1
        fi
        
        # Set OS_CLIENT_CONFIG_FILE environment variable
        export OS_CLIENT_CONFIG_FILE="${CREDS_FILE}"
        
        # Read OpenStack resource files from SHARED_DIR if they exist
        # These environment variables are used by e2e-common.sh
        if [ -f "${SHARED_DIR}/HIVE_FIP_API" ]; then
            export API_FLOATING_IP=$(<"${SHARED_DIR}/HIVE_FIP_API")
        fi
        if [ -f "${SHARED_DIR}/HIVE_FIP_INGRESS" ]; then
            export INGRESS_FLOATING_IP=$(<"${SHARED_DIR}/HIVE_FIP_INGRESS")
        fi
        if [ -f "${SHARED_DIR}/OPENSTACK_EXTERNAL_NETWORK" ]; then
            export EXTERNAL_NETWORK=$(<"${SHARED_DIR}/OPENSTACK_EXTERNAL_NETWORK")
        fi
        if [ -f "${SHARED_DIR}/OPENSTACK_COMPUTE_FLAVOR" ]; then
            export COMPUTE_FLAVOR=$(<"${SHARED_DIR}/OPENSTACK_COMPUTE_FLAVOR")
        fi
        if [ -f "${SHARED_DIR}/OPENSTACK_CONTROLPLANE_FLAVOR" ]; then
            export CONTROLPLANE_FLAVOR=$(<"${SHARED_DIR}/OPENSTACK_CONTROLPLANE_FLAVOR")
        fi
        if [ -f "${SHARED_DIR}/HIVE_CLUSTER_NAME" ]; then
            export CLUSTER_NAME=$(<"${SHARED_DIR}/HIVE_CLUSTER_NAME")
        fi
        ;;
    "nutanix")
        # TODO: Implement Nutanix credential setup
        # Nutanix uses environment variables:
        #   - NUTANIX_USERNAME: Prism Central username
        #   - NUTANIX_PASSWORD: Prism Central password
        #   - NUTANIX_HOST: Prism Central address
        #   - NUTANIX_PORT: Prism Central port (default: 9440)
        #   - PE_HOST: Prism Element address
        #   - PE_PORT: Prism Element port (default: 9440)
        #   - PE_UUID: Prism Element UUID
        #   - PE_NAME: Prism Element name
        #   - SUBNET_UUID: Subnet UUIDs
        #   - NUTANIX_AZ_NAME: Availability Zone name
        #   - API_VIP: API virtual IP address
        #   - INGRESS_VIP: Ingress virtual IP address
        #   - NUTANIX_CERT: CA certificates path (optional)
        # Credentials can be read from ~/.nutanix/credentials file
        # See e2e-common.sh for required environment variables
        echo "Note: Nutanix credential setup is not yet implemented" >&2
        echo "Please set required environment variables manually or refer to e2e-common.sh" >&2
        CREDS_FILE="(TODO: not implemented)"
        ;;
    "ibmcloud")
        # IBM Cloud: Uses environment variable IC_API_KEY
        if [ -z "$IC_API_KEY" ]; then
            echo "IC_API_KEY environment variable is not set"
            echo "Please enter your IBM Cloud API key:"
            read -r IC_API_KEY
            if [ -z "$IC_API_KEY" ]; then
                echo "Error: IC_API_KEY cannot be empty" >&2
                exit 1
            fi
        else
            echo "Using IBM Cloud API key from environment variable IC_API_KEY"
        fi
        # Export the variable so it's available to child processes
        export IC_API_KEY="$IC_API_KEY"
        ;;
    "vsphere")
        # TODO: Implement vSphere credential setup
        # See e2e-common.sh for required environment variables
        echo "Note: vSphere credential setup is not yet implemented" >&2
        echo "Please set required environment variables manually or refer to e2e-common.sh" >&2
        CREDS_FILE="(TODO: not implemented)"
        ;;
    *)
        echo "Warning: Unknown cloud platform: ${CLOUD}" >&2
        echo "Credential setup may not be configured for this platform" >&2
        CREDS_FILE="(not configured)"
        ;;
    esac
}

# Get default BASE_DOMAIN based on cloud platform
# Requires CLOUD to be set (should be called after select_cloud_platform)
function get_default_base_domain() {
    if [ -z "$CLOUD" ]; then
        echo "Error: CLOUD is not set. Please call select_cloud_platform() first." >&2
        return 1
    fi
    case "$CLOUD" in
    "aws")
        echo "hive-ci.devcluster.openshift.com"
        ;;
    "azure")
        echo "ci.azure.devcluster.openshift.com"
        ;;
    "gcp")
        echo "origin-ci-int-gce.dev.openshift.com"
        ;;
    "vsphere")
        echo "vmc.devcluster.openshift.com"
        ;;
    "openstack")
        echo "shiftstack.devcluster.openshift.com"
        ;;
    "ibmcloud")
        echo "hive-ci.devcluster.openshift.com"
        ;;
    "nutanix")
        echo "hive-ci.devcluster.openshift.com"
        ;;
    *)
        echo ""
        ;;
    esac
}

# Setup BASE_DOMAIN interactively if not set
# Requires CLOUD to be set (should be called after select_cloud_platform)
function setup_base_domain() {
    if [ -z "$CLOUD" ]; then
        echo "Error: CLOUD is not set. Please call select_cloud_platform() first." >&2
        exit 1
    fi
    if [ -z "$BASE_DOMAIN" ]; then
        DEFAULT_BASE_DOMAIN=$(get_default_base_domain)
        if [ -n "$DEFAULT_BASE_DOMAIN" ]; then
            echo "BASE_DOMAIN environment variable is not set"
            echo "Please enter the base domain for the e2e cluster [default: ${DEFAULT_BASE_DOMAIN}]:"
            read -r BASE_DOMAIN
            BASE_DOMAIN="${BASE_DOMAIN:-$DEFAULT_BASE_DOMAIN}"
            echo "Using BASE_DOMAIN: $BASE_DOMAIN"
        else
            echo "BASE_DOMAIN environment variable is not set"
            echo "Please enter the base domain for the e2e cluster:"
            read -r BASE_DOMAIN
            if [ -z "$BASE_DOMAIN" ]; then
                echo "Error: BASE_DOMAIN cannot be empty" >&2
                exit 1
            fi
        fi
    fi
    export BASE_DOMAIN="$BASE_DOMAIN"
}

# Setup common environment variables
function setup_common_variables() {
    # Namespace where e2e cluster will be created
    export CLUSTER_NAMESPACE="${CLUSTER_NAMESPACE:-hive-e2e}"

    # Where artifacts from the test will be placed
    export ARTIFACT_DIR="/tmp/hive-e2e-artifacts"

    # Public SSH key for the cluster
    export SSH_PUBLIC_KEY_FILE="${SSH_PUBLIC_KEY_FILE:-${HOME}/.ssh/id_rsa.pub}"

    # Pull secret to use for installing the cluster
    export PULL_SECRET_FILE="${PULL_SECRET_FILE:-${HOME}/.pull-secret}"
}

# Print configuration summary
# This function can be overridden in the calling script for custom output
# Or use CONFIG_TITLE environment variable to customize the title
function print_config() {
    local title="${CONFIG_TITLE:-Configuration Summary}"
    echo "=========================================="
    echo "$title"
    echo "=========================================="
    echo "KUBECONFIG: $KUBECONFIG"
    echo "CLOUD: $CLOUD"
    echo "CLUSTER_PROFILE_DIR: $CLUSTER_PROFILE_DIR"
    if [ -n "$SHARED_DIR" ] && [ "$SHARED_DIR" != "$CLUSTER_PROFILE_DIR" ]; then
        echo "SHARED_DIR: $SHARED_DIR"
    fi
    if [ -n "$CREDS_FILE" ]; then
        echo "CREDS_FILE: $CREDS_FILE"
    fi
    echo "HIVE_IMAGE: $HIVE_IMAGE"
    echo "RELEASE_IMAGE: $RELEASE_IMAGE"
    echo "CLUSTER_NAMESPACE: $CLUSTER_NAMESPACE"
    echo "BASE_DOMAIN: $BASE_DOMAIN"
    echo "ARTIFACT_DIR: $ARTIFACT_DIR"
    echo "SSH_PUBLIC_KEY_FILE: $SSH_PUBLIC_KEY_FILE"
    echo "PULL_SECRET_FILE: $PULL_SECRET_FILE"
    echo "=========================================="
    echo ""
}
