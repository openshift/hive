#!/usr/bin/env bash

# TODO: hiveadmission support
# TODO: run hive-controllers/hive-clustersync separately with multiple replicas each (need docker to avoid port collisions)

set -e
set -u
set -o pipefail

# Highlighted echo
function hl_echo {
    tput setaf 3
    echo "$@"
    tput sgr0
}

function usage {
    echo "The script runs a Hive component ($hive_component_operator or $hive_component_controllers) locally
against the active cluster, simulating in-cluster environment. It captures the in-cluster Hive
configurations, scales down the selected component, then runs it locally with the same environment.
On pressing ctrl + c, the script is terminated and relevant components are scaled back up.

Warnings:
- The active cluster might be in use, in which case the script could interfere with its functioning.

Notes:
- Artifacts produced are stored in the --artifacts_folder which is removed once the script ends unless
--preserve-artifacts is specified.
- Selecting $hive_component_controllers runs all Hive controllers (including $hive_component_clustersync) together
as a single replica.
- Endpoints (e.g. metrics, pprof) of the Hive component running locally are directly exposed on localhost.

Usage:
- ./hack/run-hive-locally.sh COMPONENT [--flag=value ...]

Parameters:
- COMPONENT: must be either $hive_component_operator or $hive_component_controllers

Optional flags:
- --hive-operator-ns=hive: namespace where hive-operator is installed.
- --log-level=debug: log level of the component to run.
- --artifacts-folder=artifacts: name of the folder that will be created for artifacts.
- --preserve-artifacts: if specified, preserve the artifacts folder after the script ends.
- --help"
}

function kill_background_jobs {
    local pid
    while read -r pid; do
        hl_echo "Killing process $pid"
        kill "$pid" || true
    done < <(jobs -p)
    wait
}

function get_envs {
    local type_name="$1"
    local ns="$2"
    # Arrange output into the following format:
    # name1 value1
    # name2 value2
    # ...
    local jsonpath_query='{range .spec.template.spec.containers[0].env[*]}{@.name} {@.value}{"\n"}{end}'
    local env
    local value
    local resolved_value

    hl_echo "Getting environment variables from $type_name in ns/$ns"
    while read -r env value; do
        # Skip environment variables that are already resolved
        if [ ${resolved_envs["$env"]+_} ]; then
            hl_echo "Environment variable $env is already resolved, skipping"
            continue
        fi

        # Skip irrelevant environment variables
        if [[ $env = "CLI_CACHE_DIR" ]] || [[ $env = "HIVE_SKIP_LEADER_ELECTION" ]]; then
            hl_echo "Environment variable $env is irrelevant, skipping"
            continue
        fi

        # Resolve the environment variable's value
        case "$value" in
        /*)
            resolved_value="./$artifacts_folder/${hc_spec_generation}$(tr / - <<< "$value").txt"
            oc exec "$type_name" -n "$ns" -- /bin/bash -c "[ -f $value ] && cat $value || echo -n '!ENOENT!'" > "$resolved_value"
            if [ '!ENOENT!' == "$(cat "$resolved_value")" ]; then
                rm "$resolved_value"
            else
                hl_echo "File content saved to $resolved_value"
            fi
            ;;
        *)
            resolved_value="$value"
            ;;
        esac

        resolved_envs["$env"]="$env=$resolved_value"
        hl_echo "Environment variable $env=$resolved_value"
    done < <(oc get "$type_name" -n "$ns" -o jsonpath="$jsonpath_query")
}

function get_envs_from {
    local type_name="$1"
    local ns="$2"
    # Arrange output into the following format:
    # ConfigMap1
    # ConfigMap2
    # ...
    local jsonpath_query='{range .spec.template.spec.containers[0].envFrom[*]}{@.configMapRef.name}{"\n"}{end}'
    # Arrange output into the following format:
    # key1 value1
    # key2 value2
    # ...
    local jq_query='to_entries[] | "\(.key) \(.value)"'
    local cm
    local env
    local value
    hl_echo "Getting envFroms from $type_name in ns/$ns"

    while read -r cm; do
        # Skip ConfigMaps that are already resolved
        if [ ${resolved_envFrom_cms["$cm"]+_} ]; then
            hl_echo "cm/$cm already resolved, skipping"
            continue
        fi

        # Extract environment variables from ConfigMap
        hl_echo "Extracting environment variables from cm/$cm in ns/$ns"
        while read -r env value; do
            resolved_envs["$env"]="$env=$value"
            hl_echo "Environment variable $env=$value"
        done < <(oc get "cm/$cm" -n "$ns" -o jsonpath="{.data}" | jq -r "$jq_query")

        # Mark ConfigMap as resolved
        resolved_envFrom_cms["$cm"]=""

        # Also save ConfigMap.data for future reference
        oc get "cm/$cm" -n "$ns" -o jsonpath="{.data}" > "./$artifacts_folder/${hc_spec_generation}-cm-$cm.txt"
    done < <(oc get "$type_name" -n "$ns" -o jsonpath="$jsonpath_query")
}

function get_incluster_config {
    # Get current HiveConfig.spec which will be compared against later to see if this field has changed
    local hc_spec_file="./$artifacts_folder/${hc_spec_generation}-hiveconfig-spec.txt"
    hiveconfig_spec="$(oc get hiveconfig hive -o jsonpath='{.spec}' | jq '.maintenanceMode = true' --sort-keys)"
    echo "$hiveconfig_spec" > "$hc_spec_file"
    hl_echo "Current HiveConfig.spec saved to $hc_spec_file"

    # Get envs and ConfigMaps corresponding to the HiveConfig.spec obtained above
    local type_names=("deploy/$hive_component_controllers" "sts/$hive_component_clustersync")
    local type_name
    for type_name in "${type_names[@]}"; do
        hl_echo "Getting in-cluster configurations from $type_name in ns/$hive_controllers_ns"
        get_envs "$type_name" "$hive_controllers_ns"
        get_envs_from "$type_name" "$hive_controllers_ns"
    done
}

function wait_until_hiveconfig_spec_changes {
    local new_hiveconfig_spec
    while read -r; do
        new_hiveconfig_spec="$(oc get hiveconfig hive -o jsonpath='{.spec}' | jq)"
        # As "oc get -w" gives spurious outputs sometimes, here we check if HiveConfig.spec is actually updated
        if [[ "$new_hiveconfig_spec" != "$hiveconfig_spec" ]]; then
            break
        fi
    done < <(oc get hiveconfig hive --watch-only --no-headers)

    hc_spec_generation=$(( hc_spec_generation + 1 ))
    hive_controllers_ns="$(oc get hiveconfig hive -o jsonpath='{.spec.targetNamespace}')"
    hl_echo "Changes to HiveConfig.spec detected (new version on the left):"
    hl_echo "$(diff -y <(echo "$new_hiveconfig_spec" ) <(echo "$hiveconfig_spec"))"
    hl_echo "Current HiveConfig.spec generation = $hc_spec_generation"
    hl_echo "Current Hive controllers namespace = $hive_controllers_ns"
}

function enable_maintenance_mode {
    hl_echo "Enabling maintenance mode"
    oc patch hiveconfig hive -p '{"spec": {"maintenanceMode": true}}' --type merge

    hl_echo "Waiting for the controllers to be scaled down"
    # Blocks until the existing hive-controllers is updated to have 0 replica
    oc wait -n "$hive_controllers_ns" --for=jsonpath='{.spec.replicas}'=0 deploy/"$hive_component_controllers"
    # Blocks until the new hive-clustersync is created and has zero replica
    while ! oc wait -n "$hive_controllers_ns" --for=jsonpath='{.spec.replicas}'=0 sts/"$hive_component_clustersync"; do
        sleep 1
    done
}

function disable_maintenance_mode {
    hl_echo "Disabling maintenance mode"
    oc patch hiveconfig hive -p '{"spec": {"maintenanceMode": false}}' --type merge

    hl_echo "Waiting for the controllers to be scaled up"
    # Blocks until the existing hive-controllers is updated to have 1 replica
    while ! oc wait -n "$hive_controllers_ns" --for=jsonpath='{.status.updatedReplicas}'=1 deploy/"$hive_component_controllers"; do
        sleep 1
    done
    # Blocks until the new hive-clustersync is created and has the correct number of replicas
    while ! oc wait -n "$hive_controllers_ns" --for=jsonpath='{.status.updatedReplicas}'="$hive_clustersync_replicas" sts/"$hive_component_clustersync"; do
        sleep 1
    done
}

function run_hive_operator_locally {
    hl_echo "Scaling down Deployment/hive-operator"
    oc scale -n "$hive_operator_ns" deploy/"$hive_component_operator" --replicas=0
    oc wait -n "$hive_operator_ns" --for=jsonpath='{.spec.replicas}'=0 deploy/"$hive_component_operator"

    hl_echo "Running $component locally, press ctrl + c when you are done"
    local final_env_values=(
        HIVE_OPERATOR_NS="$hive_operator_ns" \
        LOG_LEVEL="$log_level"
    )
    env "${final_env_values[@]}" make run-operator 2>&1 | tee "./$artifacts_folder/${hive_component_operator}-logs.txt"
}

function run_hive_controllers_locally {
    while true; do
        # Declare (global) associative arrays
        declare -gA resolved_envs resolved_envFrom_cms

        # Get in-cluster configurations
        get_incluster_config
        resolved_envs["HIVE_NS"]="HIVE_NS=$hive_controllers_ns"
        resolved_envs["HIVE_CLUSTERSYNC_POD_NAME"]="HIVE_CLUSTERSYNC_POD_NAME=hive-clustersync-0"
        resolved_envs["HIVE_MACHINEPOOL_POD_NAME"]="HIVE_MACHINEPOOL_POD_NAME=hive-machinepool-0"

        # Enable maintenance mode and wait for the controllers to be scaled down
        enable_maintenance_mode

        # Run hive-controllers as a background job
        hl_echo "Running $hive_component_controllers locally, press ctrl + c when you are done"
        # The "build" dependency of the "run" make target is only needed for the first iteration,
        # i.e. when hc_spec_generation is zero.
        if [ $hc_spec_generation -eq 0 ]; then
            env "${resolved_envs[@]}" LOG_LEVEL="$log_level" make run 2>&1 |
            tee "./$artifacts_folder/${hc_spec_generation}-${hive_component_controllers}-logs.txt" &
        else
            env "${resolved_envs[@]}" ./bin/manager --log-level="$log_level" 2>&1 |
            tee "./$artifacts_folder/${hc_spec_generation}-${hive_component_controllers}-logs.txt" &
        fi

        # The controllers running locally are stopped once HiveConfig.spec changes
        # They will be re-started with the latest configurations in the next iteration
        wait_until_hiveconfig_spec_changes
        kill_background_jobs

        # Disable maintenance mode and wait for the controllers to be scaled up
        # We can thus get Hive configurations with "oc exec" in the next iteration
        disable_maintenance_mode
        unset resolved_envs resolved_envFrom_cms
    done
}

function restore_and_cleanup {
    hl_echo $'\nCleaning up before exit'

    # Clear trap
    trap '' EXIT SIGINT

    # Restore Hive
    case "${component}" in
    "$hive_component_operator")
        hl_echo "Scaling up Deployment/hive-operator"
        oc scale -n "$hive_operator_ns" deploy/hive-operator --replicas="$hive_operator_replicas"
        ;;
    "$hive_component_controllers")
        kill_background_jobs
        disable_maintenance_mode
        ;;
    esac

    # Clean up artifacts
    if [[ -d "./$artifacts_folder" ]] && [[ "$cleanup_artifacts" = "true" ]]; then
        hl_echo "Cleaning up artifacts"
        rm -r "$artifacts_folder"
    fi

    # Propagate the exit signal to stop script execution
    exit 0
}

# Defaults
hive_component_operator="hive-operator"
hive_component_controllers="hive-controllers"
hive_component_clustersync="hive-clustersync"
hive_operator_ns="hive"
log_level="debug"
artifacts_folder="artifacts"
cleanup_artifacts="true"
hc_spec_generation=0

# Parse parameters
component="${1:-}"
case "$component" in
"$hive_component_operator"|"$hive_component_controllers")
    shift
    ;;
*)
    hl_echo "Invalid component: \"$component\"" >&2
    usage
    exit 1
    ;;
esac

# Parse options
while [ $# -gt 0 ]; do
    case "$1" in
    "--hive-operator-ns="*)
        hive_operator_ns="${1#*=}"
        ;;
    "--log-level="*)
        log_level="${1#*=}"
        ;;
    "--artifacts-folder="*)
        artifacts_folder="${1#*=}"
        ;;
    "--preserve-artifacts")
        cleanup_artifacts="false"
        ;;
    "--help")
        usage
        exit 0
        ;;
    *)
        hl_echo "Invalid option: \"$1\"" >&2
        usage
        exit 1
        ;;
    esac
    shift
done

# Exit if the artifacts folder already exists
if [[ -d "./$artifacts_folder" ]]; then
    hl_echo "Artifacts folder $artifacts_folder already exists, please remove it before running the script. "
    exit 1
fi

# Queries
hive_controllers_ns="$(oc get hiveconfig hive -o jsonpath='{.spec.targetNamespace}')"
hive_controllers_ns="${hive_controllers_ns:-hive}"
hive_operator_replicas="$(oc get "deploy/$hive_component_operator" -n "$hive_operator_ns" -o jsonpath='{.spec.replicas}')"
hive_clustersync_replicas="$(oc get "sts/$hive_component_clustersync" -n "$hive_controllers_ns" -o jsonpath='{.spec.replicas}')"

# Set trap
trap restore_and_cleanup EXIT SIGINT

# Create artifacts folder
hl_echo "Creating artifacts folder ./$artifacts_folder"
mkdir "$artifacts_folder"

# Run the selected component locally
hl_echo "Component to run locally: $component"
case "$component" in
"$hive_component_operator")
    run_hive_operator_locally
    ;;
"$hive_component_controllers")
    run_hive_controllers_locally
    ;;
esac
