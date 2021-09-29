#!/usr/bin/env python

import argparse
import datetime
import git
import github as gh
import json
import os
import shutil
import subprocess
import sys
import tempfile
import urllib3
import yaml
import re

# HIVE_VERSION_PREFIX is the prefix of the Hive version that will be constructed by this script
# the version used for images and bundles will be "{prefix}.{number of commits}-{git hash}" eg. "v1.2.3187-18827f6"
HIVE_VERSION_PREFIX = "1.2"

HIVE_REPO = "git@github.com:openshift/hive.git"

# Hive dir within both:
# https://github.com/redhat-openshift-ecosystem/community-operators-prod
# https://github.com/k8s-operatorhub/community-operators
HIVE_SUB_DIR = "operators/hive-operator"

OPERATORHUB_HIVE_IMAGE_DEFAULT = "quay.io/openshift-hive/hive"

REGISTRY_AUTH_FILE_DEFAULT = "{}/.docker/config.json".format(os.environ["HOME"])

COMMUNITY_OPERATORS_HIVE_PKG_URL = "https://raw.githubusercontent.com/redhat-openshift-ecosystem/community-operators-prod/main/operators/hive-operator/hive.package.yaml"

CHANNEL_DEFAULT = "alpha"
HIVE_BRANCH_DEFAULT = "master"

SUBPROCESS_REDIRECT = subprocess.DEVNULL


def get_params():
    parser = argparse.ArgumentParser(
        description="Hive Bundle Generator and Publishing Script"
    )
    parser.add_argument(
        "--verbose",
        default=False,
        help="Show more details while running",
        action="store_true",
    )
    parser.add_argument(
        "--dry-run",
        default=False,
        help="Test run that skips pushing branches and submitting PRs",
        action="store_true",
    )
    parser.add_argument(
        "--branch",
        default=HIVE_BRANCH_DEFAULT,
        help="""The branch (or commit-ish) from which to build and push the image.
                                If unspecified, we assume `{}`. If we're using `{}`, we
                                generate the bundle version number based on the hive version prefix `{}` and
                                update the `{}` channel. If BRANCH *also* corresponds to an `ocm-X.Y`,
                                we'll also update that channel with the same bundle. If BRANCH corresponds
                                to an `ocm-X.Y` but *not* `{}`, we'll generate the bundle version
                                number based on `X.Y` and update only that channel.""".format(
            HIVE_BRANCH_DEFAULT,
            HIVE_BRANCH_DEFAULT,
            HIVE_VERSION_PREFIX,
            CHANNEL_DEFAULT,
            HIVE_BRANCH_DEFAULT,
        ),
    )
    parser.add_argument(
        "--registry-auth-file",
        default=REGISTRY_AUTH_FILE_DEFAULT,
        help="Path to registry auth file",
    )
    parser.add_argument(
        "--github-user",
        default=os.environ["USER"],
        help="User's github username, if different than $USER",
    )
    parser.add_argument(
        "--hold",
        default=False,
        help='Adds a /hold comment in commit body to prevent the PR from merging (use "/hold cancel" to remove)',
        action="store_true",
    )
    args = parser.parse_args()

    if not os.path.isfile(args.registry_auth_file):
        parser.error(
            "--registry-auth-file ({}) does not exist, provide --registry-auth-file".format(
                args.registry_auth_file
            )
        )

    if args.verbose:
        global SUBPROCESS_REDIRECT
        SUBPROCESS_REDIRECT = None

    return args


# build_and_push_image uses buildah to build the HIVE image (tagged with "v{hive_version}"
# eg. "v1.2.3187-18827f6") and then pushes the image to quay
def build_and_push_image(registry_auth_file, hive_version, dry_run):
    container_name = "{}:v{}".format(OPERATORHUB_HIVE_IMAGE_DEFAULT, hive_version)

    if dry_run:
        print("Skipping build of container {} due to dry-run".format(container_name))
        return

    # Did we already build it locally?
    cp = subprocess.run(
        "buildah images -nq {}".format(container_name).split(),
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )
    if cp.returncode == 0:
        print(
            "Container {} already exists locally; not rebuilding.".format(
                container_name
            )
        )
    else:
        # build/push the thing
        print("Building container {}".format(container_name))

        cmd = "buildah bud --tag {} -f ./Dockerfile".format(container_name).split()
        subprocess.run(cmd)

    print("Pushing container")
    cmd = "buildah push "
    if registry_auth_file != None:
        cmd = cmd + " --authfile={} ".format(registry_auth_file)
    cmd = cmd + " {}".format(container_name)
    subprocess.run(cmd.split())


# gen_hive_version generates and returns the hive version eg. "1.2.3187-18827f6"
def gen_hive_version(repo, commit_hash, prefix):
    # sha is the first 7 characters of commit_hash
    sha = commit_hash[0:7]

    # num commits is the number of git commits counted from the first commit to the provided commit_hash
    # this is the equivalent of running:
    # git rev-list `git rev-list --parents HEAD | egrep "^[a-f0-9]{40}$"`..HEAD --count
    parent = repo.git.rev_list("--max-parents=0", commit_hash)
    num_commits = repo.git.rev_list(
        "--count",
        "{parent}..{commit_hash}".format(parent=parent, commit_hash=commit_hash),
    )

    return "{prefix}.{commits}-{sha}".format(
        prefix=prefix, commits=num_commits, sha=sha
    )


# get_previous_version grabs the previous hive version (without the leading `v`) from
# COMMUNITY_OPERATORS_HIVE_PKG_URL package yaml for the provided channel_name.
def get_previous_version(channel_name):
    http = urllib3.PoolManager()
    r = http.request("GET", COMMUNITY_OPERATORS_HIVE_PKG_URL)
    pkg_yaml = yaml.load(r.data.decode("utf-8"), Loader=yaml.FullLoader)
    try:
        for channel in pkg_yaml["channels"]:
            if channel["name"] == channel_name:
                return channel["currentCSV"].replace("hive-operator.v", "")
    except:
        print(
            "Unable to determine previous hive version from {}",
            COMMUNITY_OPERATORS_HIVE_PKG_URL,
        )
        raise


def generate_csv_base(bundle_dir, version, prev_version, channel):
    if version == prev_version:
        raise ValueError("Version {} already exists upstream".format(version))
    print("Writing bundle files to directory: %s" % bundle_dir)
    print("Generating CSV for version: %s" % version)

    crds_dir = "config/crds"
    csv_template = "config/templates/hive-csv-template.yaml"
    operator_role = "config/operator/operator_role.yaml"
    deployment_spec = "config/operator/operator_deployment.yaml"
    package_file = os.path.join(bundle_dir, "hive.package.yaml")

    # The bundle directory doesn't have the 'v'
    version_dir = os.path.join(bundle_dir, version)
    if not os.path.exists(version_dir):
        os.mkdir(version_dir)

    owned_crds = []

    # Copy all CSV files over to the bundle output dir:
    crd_files = sorted(os.listdir(crds_dir))
    for file_name in crd_files:
        full_path = os.path.join(crds_dir, file_name)
        if os.path.isfile(os.path.join(crds_dir, file_name)):
            dest_path = os.path.join(version_dir, file_name)
            shutil.copy(full_path, dest_path)
            # Read the CRD yaml to add to owned CRDs list
            with open(dest_path, "r") as stream:
                crd_csv = yaml.load(stream, Loader=yaml.SafeLoader)
                owned_crds.append(
                    {
                        "description": crd_csv["spec"]["versions"][0]["schema"][
                            "openAPIV3Schema"
                        ]["description"],
                        "displayName": crd_csv["spec"]["names"]["kind"],
                        "kind": crd_csv["spec"]["names"]["kind"],
                        "name": crd_csv["metadata"]["name"],
                        "version": crd_csv["spec"]["versions"][0]["name"],
                    }
                )

    with open(csv_template, "r") as stream:
        csv = yaml.load(stream, Loader=yaml.SafeLoader)

    csv["spec"]["customresourcedefinitions"]["owned"] = owned_crds

    csv["spec"]["install"]["spec"]["clusterPermissions"] = []

    # Add our operator role to the CSV:
    with open(operator_role, "r") as stream:
        operator_role = yaml.load(stream, Loader=yaml.SafeLoader)
        csv["spec"]["install"]["spec"]["clusterPermissions"].append(
            {"rules": operator_role["rules"], "serviceAccountName": "hive-operator",}
        )

    # Add our deployment spec for the hive operator:
    with open(deployment_spec, "r") as stream:
        operator_components = []
        operator = yaml.load_all(stream, Loader=yaml.SafeLoader)
        for doc in operator:
            operator_components.append(doc)
        operator_deployment = operator_components[1]
        csv["spec"]["install"]["spec"]["deployments"][0]["spec"] = operator_deployment[
            "spec"
        ]

    # Update the versions to include git hash:
    csv["metadata"]["name"] = "hive-operator.v%s" % version
    csv["spec"]["version"] = version
    csv["spec"]["replaces"] = "hive-operator.v%s" % prev_version

    # Update the deployment to use the defined image:
    image_ref = "%s:v%s" % (OPERATORHUB_HIVE_IMAGE_DEFAULT, version)
    csv["spec"]["install"]["spec"]["deployments"][0]["spec"]["template"]["spec"][
        "containers"
    ][0]["image"] = image_ref
    csv["metadata"]["annotations"]["containerImage"] = image_ref

    # Set the CSV createdAt annotation:
    now = datetime.datetime.now()
    csv["metadata"]["annotations"]["createdAt"] = now.strftime("%Y-%m-%dT%H:%M:%SZ")

    # Write the CSV to disk:
    csv_filename = "hive-operator.v%s.clusterserviceversion.yaml" % version
    csv_file = os.path.join(version_dir, csv_filename)
    with open(csv_file, "w") as outfile:
        yaml.dump(csv, outfile, default_flow_style=False)
    print("Wrote ClusterServiceVersion: %s" % csv_file)

    # generate package
    generate_package(package_file, channel, version)


def generate_package(package_file, channel, version):
    document_template = """
      channels:
      - currentCSV: %s
        name: %s
      defaultChannel: %s
      packageName: hive-operator
"""
    name = "hive-operator.v%s" % version
    document = document_template % (name, channel, channel)

    with open(package_file, "w") as outfile:
        yaml.dump(
            yaml.load(document, Loader=yaml.SafeLoader),
            outfile,
            default_flow_style=False,
        )
    print("Wrote package: %s" % package_file)


def open_pr(
    work_dir,
    fork_repo,
    upstream_repo,
    gh_username,
    bundle_source_dir,
    new_version,
    update_channels,
    hold,
    dry_run,
):

    dir_name = fork_repo.split("/")[1][:-4]

    dest_github_org = upstream_repo.split(":")[1].split("/")[0]
    dest_github_reponame = dir_name

    os.chdir(work_dir)

    print()
    print()
    print("Cloning %s" % fork_repo)
    repo_full_path = os.path.join(work_dir, dir_name)
    # clone git repo
    try:
        git.Repo.clone_from(fork_repo, repo_full_path)
    except:
        print("Failed to clone repo {} to {}".format(fork_repo, repo_full_path))
        raise

    # get to the right place on the filesystem
    print("Working in %s" % repo_full_path)
    os.chdir(repo_full_path)

    repo = git.Repo(repo_full_path)
    try:
        repo.create_remote("upstream", upstream_repo)
    except:
        print("Failed to create upstream remote")
        raise

    print("Fetching latest upstream")
    try:
        repo.remotes.upstream.fetch()
    except:
        print("Failed to fetch upstream")
        raise

    # Starting branch
    print("Checkout latest upstream/main")
    try:
        repo.git.checkout("upstream/main")
    except:
        print("Failed to checkout upstream/main")
        raise

    branch_name = "update-hive-{}".format(new_version)
    pr_title = "Update Hive community operator to {}".format(new_version)
    print("Starting {}".format(pr_title))

    print("Create branch {}".format(branch_name))
    try:
        repo.git.checkout("-b", branch_name)
    except:
        print("Failed to checkout branch {}".format(branch_name))
        raise

    # copy bundle directory
    print("Copying bundle directory")
    bundle_files = os.path.join(bundle_source_dir, new_version)
    hive_dir = os.path.join(repo_full_path, HIVE_SUB_DIR, new_version)
    shutil.copytree(bundle_files, hive_dir)

    # update bundle manifest
    print("Updating bundle manfiest")
    bundle_manifests_file = os.path.join(
        repo_full_path, HIVE_SUB_DIR, "hive.package.yaml"
    )
    bundle = {}
    with open(bundle_manifests_file, "r") as a_file:
        bundle = yaml.load(a_file, Loader=yaml.SafeLoader)

    found = False
    for channel in bundle["channels"]:
        if channel["name"] in update_channels:
            found = True
            channel["currentCSV"] = "hive-operator.{}".format(new_version)

    if not found:
        print("did not find a CSV channel to update")
        sys.exit(1)

    with open(bundle_manifests_file, "w") as outfile:
        yaml.dump(bundle, outfile, default_flow_style=False)
    print("\nUpdated bundle package:\n\n")
    cmd = ("cat %s" % bundle_manifests_file).split()
    subprocess.run(cmd)
    print()

    # commit files
    print("Adding file")
    repo.git.add(HIVE_SUB_DIR)

    print("Committing {}".format(pr_title))
    try:
        repo.git.commit("--signoff", "--message={}".format(pr_title))
    except:
        print("Failed to commit")
        raise
    print()

    if not dry_run:
        print("Pushing branch {}".format(branch_name))
        origin = repo.remotes.origin
        try:
            origin.push(branch_name, None, force=True)
        except:
            print("failed to push branch to origin")
            raise

        # open PR
        client = gh.GitHubClient(dest_github_org, dest_github_reponame, "")

        from_branch = "{}:{}".format(gh_username, branch_name)
        to_branch = "main"

        body = pr_title
        if hold:
            body = "%s\n\n/hold" % body

        resp = client.create_pr(from_branch, to_branch, pr_title, body)
        if resp.status_code != 201:  # 201 == Created
            print(resp.text)
            sys.exit(1)

        json_content = json.loads(resp.content.decode("utf-8"))
        print("PR opened: {}".format(json_content["html_url"]))

    else:
        print("Skipping branch push due to dry-run")
    print()


def branch_is_on_master_commit(branch, repo_dir):
    repo = git.Repo(repo_dir)

    print("Checkout out {} branch".format(branch))
    try:
        repo.git.checkout(branch)
    except:
        print("Failed to checkout branch {}".format(branch))
        raise

    return repo.rev_parse("master") == repo.rev_parse(branch)


def buildah_installed():
    return shutil.which("buildah") is not None


def process_branch(hive_repo, branch_arg):
    """Validate and process the input (or default) branch.

    :param hive_repo: The git.Repo for the local hive clone.
    :param branch_arg: The string commit-ish input (or defaulted) via the --branch arg.
    :return commit_hash: The canonical full-length sha of the commit corresponding to branch_arg.
    :return prefix: The two-digit semver prefix of the bundle version to use. If branch_arg is
        (a descendant of) master, we'll use HIVE_VERSION_PREFIX. If not, and it names an 'ocm-X.Y'
        branch, we'll use X.Y.
    :return channels: List of string channel names to update. If branch_arg is (a descendant of)
        master, this will include 'alpha'. If branch_arg names an 'ocm-X.Y' branch, it will include
        'ocm-X.Y'.
    :raise: If we get a branch_arg that's invalid (doesn't correspond to a real commit in hive_repo),
        or that's not 'ocm-X.Y' or (a descendant of) master.
    """
    # Was an ocm-X.Y branch specified?
    # TODO: Detect if a *hash corresponding to* an ocm-X.Y was specified:
    #   git branch -r | while read b; do
    #       if [[ $(git rev-parse $b) == $COMMIT_TO_MATCH ]]; then
    #           # $b is a remote branch name at the given commit
    #       fi
    #   done
    # Also:
    # We're cloning the hive repo, so there's no local branch for ocm-X.Y; but we don't want to
    # force the user to say 'origin/ocm-X.Y'. Accommodate...
    m = re.match("^(?:[^/]+/)?(ocm-(\d+\.\d+))$", branch_arg)
    if m:
        # First capture group is ocm-X.Y
        branch_arg = "origin/{}".format(m.group(1))

    # This will raise an exception if there's no such commit-ish
    commit_hash = hive_repo.rev_parse(branch_arg).hexsha
    # This had better exist
    master_hash = hive_repo.rev_parse(HIVE_BRANCH_DEFAULT).hexsha
    is_master_ancestor = hive_repo.git.rev_list(
        "--ancestry-path", "{}..{}".format(commit_hash, master_hash)
    )
    channels = []
    prefix = None
    if commit_hash == master_hash or is_master_ancestor:
        channels.append(CHANNEL_DEFAULT)
        prefix = HIVE_VERSION_PREFIX
    if m:
        # First capture group is ocm-X.Y
        channels.append(m.group(1))
        # Second capture group is just the X.Y bit. But only use it if we're not tracking master.
        if not prefix:
            prefix = m.group(2)
    if not channels or not prefix:
        raise ValueError(
            "Can't make sense of branch {}: expected {}, a direct descendant thereof, or 'ocm-X.Y'.".format(
                branch_arg, HIVE_BRANCH_DEFAULT
            )
        )
    return commit_hash, prefix, channels


if __name__ == "__main__":
    args = get_params()

    if not buildah_installed():
        print("buildah not found, please install buildah")
        sys.exit(1)

    hive_repo_dir = tempfile.TemporaryDirectory(prefix="hive-repo-")

    print("Cloning {} to {}".format(HIVE_REPO, hive_repo_dir.name))
    try:
        git.Repo.clone_from(HIVE_REPO, hive_repo_dir.name)
    except:
        print("Failed to clone repo {} to {}".format(HIVE_REPO, hive_repo_dir.name))
        raise

    hive_repo = git.Repo(hive_repo_dir.name)

    # If args.branch is master or a direct ancestor, use HIVE_VERSION_PREFIX
    # (if `git rev-list --ancestry-path ${BRANCH}..master` produces no output, it's not a direct ancestor)
    # else parse ocm-X.Y
    #    hive_version_prefix = '{}'.format(args.branch.split('-', 1)[1])
    hive_commit, hive_version_prefix, update_channels = process_branch(
        hive_repo, args.branch
    )
    # update_channels = [CHANNEL_DEFAULT]
    # if args.branch:
    #     if branch_is_on_master_commit(args.branch, hive_repo_dir.name):
    #         update_channels.append(args.branch)
    #     else:
    #         update_channels = [args.branch]

    # The channel we use when looking for stuff in the package.yaml file
    channel = (
        CHANNEL_DEFAULT if CHANNEL_DEFAULT in update_channels else update_channels[0]
    )

    bundle_dir = tempfile.TemporaryDirectory(prefix="hive-operator-bundle-")
    work_dir = tempfile.TemporaryDirectory(prefix="operatorhub-push-")

    print("Checking out {}".format(hive_commit))
    try:
        hive_repo.git.checkout(hive_commit)
    except:
        print("Failed to checkout {}".format(hive_commit))
        raise

    hive_version = gen_hive_version(hive_repo, hive_commit, hive_version_prefix)
    build_and_push_image(args.registry_auth_file, hive_version, args.dry_run)

    # TODO: We shouldn't need channel here, because the package.yaml file is being updated in open_pr (right????)
    generate_csv_base(
        bundle_dir.name, hive_version, get_previous_version(channel), channel
    )

    # redhat-openshift-ecosystem/community-operators-prod
    open_pr(
        work_dir.name,
        "git@github.com:%s/community-operators-prod.git" % args.github_user,
        "git@github.com:redhat-openshift-ecosystem/community-operators-prod.git",
        args.github_user,
        bundle_dir.name,
        hive_version,
        update_channels,
        args.hold,
        args.dry_run,
    )
    # k8s-operatorhub/community-operators
    open_pr(
        work_dir.name,
        "git@github.com:%s/community-operators.git" % args.github_user,
        "git@github.com:k8s-operatorhub/community-operators.git",
        args.github_user,
        bundle_dir.name,
        hive_version,
        update_channels,
        args.hold,
        args.dry_run,
    )

    hive_repo_dir.cleanup()
    bundle_dir.cleanup()
    work_dir.cleanup()
