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

import version

HIVE_REPO_DEFAULT = "git@github.com:openshift/hive.git"

# Hive dir within both:
# https://github.com/redhat-openshift-ecosystem/community-operators-prod
# https://github.com/k8s-operatorhub/community-operators
HIVE_SUB_DIR = "operators/hive-operator"

OPERATORHUB_HIVE_IMAGE_DEFAULT = "quay.io/openshift-hive/hive"

REGISTRY_AUTH_FILE_DEFAULT = "{}/.docker/config.json".format(os.environ["HOME"])

COMMUNITY_OPERATORS_HIVE_PKG_URL = "https://raw.githubusercontent.com/redhat-openshift-ecosystem/community-operators-prod/main/operators/hive-operator/hive.package.yaml"

SUBPROCESS_REDIRECT = subprocess.DEVNULL

HIVE_BRANCH_DEFAULT = version.HIVE_BRANCH_DEFAULT

CHANNEL_DEFAULT = version.CHANNEL_DEFAULT


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
        "--registry-auth-file",
        default=REGISTRY_AUTH_FILE_DEFAULT,
        help="Path to registry auth file",
    )
    # TODO: Validate this early! As written, if this is wrong you won't bounce until open_pr.
    parser.add_argument(
        "--github-user",
        default=os.getenv("GITHUB_USER") or os.environ["USER"],
        help="User's github username. Defaults to $GITHUB_USER, then $USER.",
    )
    parser.add_argument(
        "--hold",
        default=False,
        help='Adds a /hold comment in commit body to prevent the PR from merging (use "/hold cancel" to remove)',
        action="store_true",
    )
    parser.add_argument(
        "--hive-repo",
        default=HIVE_REPO_DEFAULT,
        help="The hive git repository to clone. E.g. save time by using a local directory (but make sure it's up to date!)",
    )
    parser.add_argument(
        "--image-repo",
        default=OPERATORHUB_HIVE_IMAGE_DEFAULT,
        help="""The image repository to which the operator image should be pushed, e.g. `quay.io/myproject/hive`.
                                (Do not include a tag; it will be generated based on the branch/commit.)""",
    )
    parser.add_argument(
        "--image-tag-override",
        help="""String to use for the image tag and CSV name (but not the CSV `version`!). By default this is generated as
                                `v$X.$Y.$count-$sha, e.g. v1.2-3456-789abcd, where $X is the major version; $Y is the minor
                                version; $count is the number of commits leading up to the requested COMMIT-ISH; and $sha is
                                the first seven digits of the commit SHA corresponding to COMMIT-ISH.""",
    )
    mutex_group = parser.add_mutually_exclusive_group()
    parser.add_argument(
        "--commit",
        metavar="COMMIT-ISH",
        help="""The commit-ish from which to build and push the image.
                                This argument may be used on its own or in conjunction with --dummy-bundle
                                or --image-only. If used as a standalone option, we generate a bundle version
                                for `{}` starting on the commit-ish provided. If --commit is not specified,
                                we assume the tip of the {} branch. If used alongside --dummy-bundle or --image-only,
                                we generate bundle files or operator image for the `mce-X.Y` branch specified,
                                at the commit-ish specified. For example, `--dummy-bundle mce-2.1 --commit $sha
                                will result in a dummy-bundle 0.0.$count-$sha for mce-2.1. For more information,
                                see the --dummy-bundle and --image-only descriptions.
                                """.format(
            HIVE_BRANCH_DEFAULT,
            HIVE_BRANCH_DEFAULT,
        ),
    )
    mutex_group.add_argument(
        "--dummy-bundle",
        metavar="BRANCH-NAME",
        help="""Only generate bundle files at a specific commit, as provided by the `--commit`
                                parameter, defaulting to the commit corresponding to BRANCH-NAME,
                                which must be `master` or a valid `mce-*` branch. The bundle files will
                                be placed in a subdirectory of your PWD named hive-operator-bundle-$version,
                                where $version is computed as '0.0.$count-$sha'; $count is the number of
                                commits leading up to the requested commit; and $sha is the first seven
                                digits of the SHA of the requested commit.
                                No image will be built. No package file will be generated. The CSV will not
                                have any graph directives (`replaces`, `skipRange`, etc.).""",
    )
    mutex_group.add_argument(
        "--image-only",
        metavar="BRANCH-NAME",
        help="""Only build and push the operator image at a specific commit, as provided by the `--commit`
                                parameter, defaulting to the commit corresponding to BRANCH-NAME,
                                which must be a `master` or a valid `mce-*` branch.
                                The $version will be computed as '0.0.$count-$sha', where
                                $count is the number of commits leading up to the requested commit; and
                                $sha is the first seven digits of the commit SHA corresponding to the requested commit.
                                No bundle or OperatorHub PRs will be generated.""",
    )
    args = parser.parse_args()

    if shutil.which("buildah"):
        args.build_engine = "buildah"
    elif shutil.which("docker"):
        args.build_engine = "docker"
    else:
        print("neither buildah nor docker found, please install one or the other.")
        sys.exit(1)

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


# build_and_push_image uses buildah or docker to build the HIVE image from the current
# working directory (tagged with "v{hive_version}" eg. "v1.2.3187-18827f6" by default,
# or the value passed to --image-tag-override if specified) and then pushes the image
# to quay.
def build_and_push_image(
    registry_auth_file, image_repo, image_tag, dry_run, build_engine
):
    container_name = "{}:{}".format(image_repo, image_tag)

    if dry_run:
        print("Skipping build of container {}".format(container_name))
        return

    if build_engine == "buildah":
        build = dict(
            query="buildah images -nq {}",
            build="buildah bud --tag {} -f ./Dockerfile",
            push="buildah push --authfile={} {}",
        )
        registry_auth_arg = registry_auth_file
    elif build_engine == "docker":
        build = dict(
            query="docker image inspect {}",
            build="docker build --tag {} -f ./Dockerfile .",
            push="docker --config {} push {}",
        )
        registry_auth_arg = os.path.dirname(registry_auth_file)

    # Did we already build it locally?
    cp = subprocess.run(
        build["query"].format(container_name).split(),
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

        cmd = build["build"].format(container_name).split()
        subprocess.run(cmd, check=True)

    print("Pushing container")
    subprocess.run(
        build["push"].format(registry_auth_arg, container_name).split(),
        check=True,
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

    # Channel not found -- no previous version.
    return None


# generate_csv_base generates a hive bundle from the current working directory
# and deposits all artifacts in the specified bundle_dir.
# If prev_version is not None/empty, the CSV will include it as `replaces`.
def generate_csv_base(bundle_dir, image_repo, version, prev_version, image_tag):
    print("Writing bundle files to directory: %s" % bundle_dir)
    print("Generating CSV for version: %s" % version)

    crds_dir = "config/crds"
    csv_template = "config/templates/hive-csv-template.yaml"
    operator_role = "config/operator/operator_role.yaml"
    deployment_spec = "config/operator/operator_deployment.yaml"

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
            {
                "rules": operator_role["rules"],
                "serviceAccountName": "hive-operator",
            }
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
    csv["metadata"]["name"] = "hive-operator.%s" % image_tag
    csv["spec"]["version"] = version
    if prev_version:
        csv["spec"]["replaces"] = "hive-operator.v%s" % prev_version

    # Update the deployment to use the defined image:
    image_ref = "%s:%s" % (image_repo, image_tag)
    csv["spec"]["install"]["spec"]["deployments"][0]["spec"]["template"]["spec"][
        "containers"
    ][0]["image"] = image_ref
    csv["metadata"]["annotations"]["containerImage"] = image_ref

    # Set the CSV createdAt annotation:
    now = datetime.datetime.now()
    csv["metadata"]["annotations"]["createdAt"] = now.strftime("%Y-%m-%dT%H:%M:%SZ")

    # Write the CSV to disk:
    csv_filename = "hive-operator.%s.clusterserviceversion.yaml" % image_tag
    csv_file = os.path.join(version_dir, csv_filename)
    with open(csv_file, "w") as outfile:
        yaml.dump(csv, outfile, default_flow_style=False)
    print("Wrote ClusterServiceVersion: %s" % csv_file)

    return version_dir


def generate_package(package_file, channel, image_tag):
    document_template = """
      channels:
      - currentCSV: %s
        name: %s
      defaultChannel: %s
      packageName: hive-operator
"""
    name = "hive-operator.%s" % image_tag
    document = document_template % (name, channel, channel)

    with open(package_file, "w") as outfile:
        yaml.dump(
            yaml.load(document, Loader=yaml.SafeLoader),
            outfile,
            default_flow_style=False,
        )
    print("Wrote package: %s" % package_file)


def copy_bundle(orig_wd, bundle_source_dir, image_tag):
    bundle_dest_dir = os.path.join(orig_wd, "hive-operator-bundle-{}".format(image_tag))
    shutil.copytree(bundle_source_dir, bundle_dest_dir)
    print("Wrote bundle to {}".format(bundle_dest_dir))


def open_pr(
    work_dir,
    fork_repo,
    upstream_repo,
    gh_username,
    bundle_source_dir,
    new_version,
    prev_version,
    image_tag,
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
        if channel["name"] == CHANNEL_DEFAULT:
            found = True
            channel["currentCSV"] = "hive-operator.{}".format(image_tag)

    if not found:
        print("did not find a CSV channel to update")
        sys.exit(1)
    pr_title = "Update Hive community operator channel {} to {}".format(
        CHANNEL_DEFAULT, new_version
    )

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


if __name__ == "__main__":
    args = get_params()

    hive_repo_dir = tempfile.TemporaryDirectory(prefix="hive-repo-")

    print("Cloning {} to {}".format(args.hive_repo, hive_repo_dir.name))
    try:
        git.Repo.clone_from(args.hive_repo, hive_repo_dir.name)
    except:
        print(
            "Failed to clone repo {} to {}".format(args.hive_repo, hive_repo_dir.name)
        )
        raise

    hive_repo = git.Repo(hive_repo_dir.name)

    if args.dummy_bundle or args.image_only:

        branch_sha = version.find_branch(
            hive_repo, args.dummy_bundle or args.image_only
        ).hexsha
        if not version.is_origin_branch(hive_repo, branch_sha):
            print(
                """Error: branch arg {} is not a valid tip of mce-* branch or master branch\n
                Consider using the `--commit {}` argument if this commit is an ancestor of a valid
                mce-* or master branch""".format(
                    args.dummy_bundle or args.image_only, branch_sha
                )
            )
            sys.exit(1)
        if args.commit:
            hive_commit = hive_repo.rev_parse(args.commit).hexsha
            if not hive_repo.is_ancestor(hive_commit, branch_sha):
                print(
                    "Commit SHA {} is not equal to or an ancestor of branch {}".format(
                        args.commit, args.dummy_bundle or args.image_only
                    )
                )
                sys.exit(1)
        else:
            hive_commit = branch_sha

        hive_version_prefix = "0.0"
    else:
        if not args.commit:
            args.commit = HIVE_BRANCH_DEFAULT
        hive_commit, hive_version_prefix = version.process_master_branch(
            hive_repo, args.commit
        )

    bundle_dir = tempfile.TemporaryDirectory(prefix="hive-operator-bundle-")
    work_dir = tempfile.TemporaryDirectory(prefix="operatorhub-push-")

    orig_wd = os.getcwd()
    print("Working in {}".format(hive_repo_dir.name))
    os.chdir(hive_repo_dir.name)

    print("Checking out {}".format(hive_commit))
    try:
        hive_repo.git.checkout(hive_commit)
    except:
        print("Failed to checkout {}".format(hive_commit))
        raise

    hive_version = version.gen_hive_version(hive_repo, hive_commit, hive_version_prefix)
    if args.dummy_bundle or args.image_only:
        # Omit version graph stuff
        prev_version = None
    else:
        prev_version = get_previous_version(CHANNEL_DEFAULT)
        if hive_version == prev_version:
            raise ValueError("Version {} already exists upstream".format(hive_version))

    image_tag = args.image_tag_override or "v{}".format(hive_version)
    build_and_push_image(
        args.registry_auth_file,
        args.image_repo,
        image_tag,
        args.dry_run or args.dummy_bundle,
        args.build_engine,
    )
    if args.image_only:
        sys.exit(0)

    version_dir = generate_csv_base(
        bundle_dir.name, args.image_repo, hive_version, prev_version, image_tag
    )

    if args.dummy_bundle:
        copy_bundle(orig_wd, version_dir, image_tag)
    else:
        generate_package(
            os.path.join(bundle_dir.name, "hive.package.yaml"),
            CHANNEL_DEFAULT,
            hive_version,
        )
        # redhat-openshift-ecosystem/community-operators-prod
        open_pr(
            work_dir.name,
            "git@github.com:%s/community-operators-prod.git" % args.github_user,
            "git@github.com:redhat-openshift-ecosystem/community-operators-prod.git",
            args.github_user,
            bundle_dir.name,
            hive_version,
            prev_version,
            image_tag,
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
            prev_version,
            image_tag,
            args.hold,
            args.dry_run,
        )

    hive_repo_dir.cleanup()
    bundle_dir.cleanup()
    work_dir.cleanup()
