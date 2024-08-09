#!/usr/bin/env python3

import argparse
import datetime
import git
import github as gh
import json
import os
import requests
import semver
import shutil
import subprocess
import sys
import tempfile
import urllib3
import yaml

import version2

HIVE_REPO_DEFAULT = "git@github.com:openshift/hive.git"

# Hive dir within both:
# https://github.com/redhat-openshift-ecosystem/community-operators-prod
# https://github.com/k8s-operatorhub/community-operators
HIVE_SUB_DIR = "operators/hive-operator"

OPERATORHUB_HIVE_IMAGE_DEFAULT = "quay.io/app-sre/hive"

COMMUNITY_OPERATORS_UPSTREAM_REPO = "git@github.com:redhat-openshift-ecosystem/community-operators-prod.git"

SUBPROCESS_REDIRECT = subprocess.DEVNULL

HIVE_BRANCH_DEFAULT = "master"

CHANNEL_DEFAULT = "alpha"


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
        help="""String to use for the image tag. By default we use the first ten digits of the sha corresponding
                                to the requested COMMIT-ISH.""",
    )
    parser.add_argument(
        "--commit",
        metavar="COMMIT-ISH",
        help="""The commit-ish from which to build and push the image.
                                This argument may be used on its own or in conjunction with --dummy-bundle.
                                If used as a standalone option, we generate a bundle version
                                for `{}` starting on the commit-ish provided. If --commit is not specified,
                                we assume the tip of the {} branch. If used alongside --dummy-bundle,
                                we generate bundle files for the `mce-X.Y` branch specified,
                                at the commit-ish specified. For example, `--dummy-bundle mce-2.1 --commit $sha
                                will result in a dummy-bundle 2.1.$count-$sha for mce-2.1. For more information,
                                see the --dummy-bundle description.
                                """.format(
            HIVE_BRANCH_DEFAULT,
            HIVE_BRANCH_DEFAULT,
        ),
    )
    parser.add_argument(
        "--dummy-bundle",
        metavar="BRANCH-NAME",
        help="""Only generate bundle files at a specific commit, as provided by the `--commit`
                                parameter, defaulting to the commit corresponding to BRANCH-NAME,
                                which must be `{}` or a valid `mce-*` branch. The bundle files will
                                be placed in a subdirectory of your PWD named hive-operator-bundle-$version,
                                where $version is computed as 'X.Y.$count-$sha'; X.Y is the MCE version
                                number, or `{}` for {}; $count is the number of commits leading up to the
                                requested commit; and $sha is the first seven digits of the SHA of the requested
                                commit. No package file will be generated. The CSV will not have any graph
                                directives (`replaces`, `skipRange`, etc.).
                                """.format(
            HIVE_BRANCH_DEFAULT,
            version2.MASTER_BRANCH_PREFIX,
            HIVE_BRANCH_DEFAULT,
        ),
    )
    parser.add_argument(
        "--skip-image-validation",
        default=False,
        help="""By default, we will check to make sure the image described by --image-repo and --image-tag-override (both
                                of which may be defaulted/computed) exists. Provide this flag to skip that validation.
                                Also note that we will currently only validate quay.io/* images.""",
        action="store_true",
    )
    args = parser.parse_args()

    if args.verbose:
        global SUBPROCESS_REDIRECT
        SUBPROCESS_REDIRECT = None

    return args

# Traverse through version directories to detect highest version
def get_previous_version(work_dir, channel_name):
    upstream_branch = "main"
    dir_name = "community-operators-prod"

    repo_full_path = os.path.join(work_dir, dir_name)
    # clone git repo
    try:
        git.Repo.clone_from(COMMUNITY_OPERATORS_UPSTREAM_REPO, repo_full_path)
    except:
        print("Failed to clone repo {} to {}".format(COMMUNITY_OPERATORS_UPSTREAM_REPO, repo_full_path))
        raise

    # get to the right place on the filesystem
    print("Working in %s" % repo_full_path)
    os.chdir(repo_full_path)

    repo = git.Repo(repo_full_path)

    # Starting branch
    print("Checkout latest {}".format(upstream_branch))
    try:
        repo.git.checkout(upstream_branch)
    except:
        print("Failed to checkout {}".format(upstream_branch))
        raise

    highest_version = "0.0.0"
    try:
        hive_dir = os.path.join(repo_full_path, HIVE_SUB_DIR)
        for version in os.listdir(hive_dir):
            annotation_yaml_path = os.path.join(hive_dir, version, "metadata", "annotations.yaml")
            try:
                with open(annotation_yaml_path, "r") as stream:
                    annotation_yaml = yaml.load(stream, Loader=yaml.SafeLoader)
                    version_channels = annotation_yaml["annotations"]["operators.operatorframework.io.bundle.channels.v1"]
                    if channel_name in version_channels.split(","):
                        if semver.compare(version, highest_version) > 0:
                            highest_version = version
            except (NotADirectoryError, FileNotFoundError):
                print("Skipping %s -- not a version", version)
                continue
    except:
        print(
            "Unable to determine previous hive version from {}",
            COMMUNITY_OPERATORS_UPSTREAM_REPO,
        )
        raise

    if highest_version == "0.0.0":
        # Channel not found -- no previous version.
        # NOTE: If a new channel is introduced, finding a prev_version will fail and require a manual edit of the CSV with the prev_version.
        # Keeping this condition fatal in order to ensure that a failure to find prev_version is noticed.
        print(
            "Channel not found. Unable to calculate determine version {}",
            COMMUNITY_OPERATORS_UPSTREAM_REPO,
        )
        sys.exit(1)

    return highest_version


# generate_csv_base generates a hive bundle from the current working directory
# and deposits all artifacts in the specified bundle_dir.
# If prev_version is not None/empty, the CSV will include it as `replaces`.
def generate_csv_base(
    bundle_dir, image_repo, v: version2.Version, prev_version, image_tag
):
    print("Writing bundle files to directory: %s" % bundle_dir)
    print("Generating CSV for version: %s" % v)

    crds_dir = "config/crds"
    csv_template = "config/templates/hive-csv-template.yaml"
    operator_role = "config/operator/operator_role.yaml"
    deployment_spec = "config/operator/operator_deployment.yaml"


    # The bundle directory doesn't have the 'v'
    version_dir = os.path.join(bundle_dir, v.semver)
    if not os.path.exists(version_dir):
        os.mkdir(version_dir)
    manifests_dir = os.path.join(version_dir, "manifests")
    if not os.path.exists(manifests_dir):
        os.mkdir(manifests_dir)

    # Create annotations.yaml file
    metadata_dir = os.path.join(version_dir, "metadata")
    if not os.path.exists(metadata_dir):
        os.mkdir(metadata_dir)
    file_path = os.path.join(metadata_dir, "annotations.yaml")
    with open(file_path, 'w') as file:
        yaml_annotations =  {
                'annotations' : {
                    'operators.operatorframework.io.bundle.channel.default.v1': CHANNEL_DEFAULT,
                    'operators.operatorframework.io.bundle.channels.v1': CHANNEL_DEFAULT,
                    'operators.operatorframework.io.bundle.manifests.v1': 'manifests/',
                    'operators.operatorframework.io.bundle.mediatype.v1': 'registry+v1',
                    'operators.operatorframework.io.bundle.metadata.v1': 'metadata/',
                    'operators.operatorframework.io.bundle.package.v1': 'hive-operator',
                }
            }
        
        yaml_string = yaml.dump(yaml_annotations)
        file.write(yaml_string)

    owned_crds = []

    # Copy all CSV files over to the manifests dir:
    crd_files = sorted(os.listdir(crds_dir))
    for file_name in crd_files:
        full_path = os.path.join(crds_dir, file_name)
        if os.path.isfile(os.path.join(crds_dir, file_name)):
            dest_path = os.path.join(manifests_dir, file_name)
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
    csv["metadata"]["name"] = "hive-operator.%s" % v
    csv["spec"]["version"] = v.semver
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
    csv_filename = "hive-operator.%s.clusterserviceversion.yaml" % v
    csv_file = os.path.join(manifests_dir, csv_filename)
    with open(csv_file, "w") as outfile:
        yaml.dump(csv, outfile, default_flow_style=False)
    print("Wrote ClusterServiceVersion: %s" % csv_file)

    return version_dir


def validate_image(image_repo, image_tag, skip):
    """Ensure the image exists.

    We only attempt to validate quay.io images.

    :param image_repo: The `host/org/repo` of the image.
    :param image_tag: The tag of the image.
    :param skip: If True, we'll skip validation.
    """
    if skip:
        print(f"Skipping validation of image {image_repo}:{image_tag}")
        return
    parsed = urllib3.util.parse_url(image_repo)
    if parsed.host != "quay.io":
        print(f"Skipping validation of non-quay image in repo {image_repo}")
        return
    url = f"https://{parsed.host}/api/v1/repository{parsed.path}/tag/?specificTag={image_tag}"
    resp = requests.get(url)
    if not resp.ok:
        print(
            f"Failed to validate image {image_repo}:{image_tag}!\nCouldn't query quay API for {url}\n\tstatus_code={resp.status_code}"
        )
        sys.exit(1)
    j = resp.json()
    if not j.get("tags"):
        print(f"No image at {image_repo}:{image_tag}!")
        sys.exit(1)

    print("Image validated successfully")


def generate_package(package_file, channel, v: version2.Version):
    document_template = """
      channels:
      - currentCSV: %s
        name: %s
      defaultChannel: %s
      packageName: hive-operator
"""
    name = "hive-operator.%s" % v
    document = document_template % (name, channel, channel)

    with open(package_file, "w") as outfile:
        yaml.dump(
            yaml.load(document, Loader=yaml.SafeLoader),
            outfile,
            default_flow_style=False,
        )
    print("Wrote package: %s" % package_file)


def copy_bundle(orig_wd, bundle_source_dir, v: version2.Version):
    bundle_dest_dir = os.path.join(orig_wd, "hive-operator-bundle-{}".format(str(v)))
    shutil.copytree(bundle_source_dir, bundle_dest_dir)
    print("Wrote bundle to {}".format(bundle_dest_dir))


def open_pr(
    work_dir,
    fork_repo,
    upstream_repo,
    gh_username,
    bundle_source_dir,
    v: version2.Version,
    hold,
    dry_run,
):
    dir_name = fork_repo.split("/")[1][:-4]

    dest_github_org = upstream_repo.split(":")[1].split("/")[0]
    dest_github_reponame = dir_name

    os.chdir(work_dir)

    print()
    print()
    repo_full_path = os.path.join(work_dir, dir_name)
    # The get_previous_version function clones the upstream RH directory
    # into community-operators-prod repo.
    # If this repo already exists, skip cloning it a second time.
    if not os.path.exists(repo_full_path):
        # clone git repo
        print("Cloning %s" % fork_repo)
        try:
            git.Repo.clone_from(fork_repo, repo_full_path)
        except:
            print("Failed to clone repo {} to {}".format(fork_repo, repo_full_path))
            raise
    else:
        print("Skipping cloning of %s. Repo already exists" % fork_repo)

    # get to the right place on the filesystem
    print("Working in %s" % repo_full_path)
    os.chdir(repo_full_path)

    repo = git.Repo(repo_full_path)

    try:
        repo.remotes.origin.set_url(fork_repo)
    except:
        print("Failed to set origin remote")
        raise

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

    branch_name = "update-hive-{}".format(v.semver)

    print("Create branch {}".format(branch_name))
    try:
        repo.git.checkout("-b", branch_name)
    except:
        print("Failed to checkout branch {}".format(branch_name))
        raise

    # copy bundle directory
    print("Copying bundle directory")
    bundle_files = os.path.join(bundle_source_dir, v.semver)
    hive_dir = os.path.join(repo_full_path, HIVE_SUB_DIR, v.semver)
    shutil.copytree(bundle_files, hive_dir)

    pr_title = "operator hive-operator ({})".format(v.semver)

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

    bundle_dir = tempfile.TemporaryDirectory(prefix="hive-operator-bundle-")
    work_dir = tempfile.TemporaryDirectory(prefix="operatorhub-push-")

    orig_wd = os.getcwd()
    print("Working in {}".format(hive_repo_dir.name))
    os.chdir(hive_repo_dir.name)

    ver = version2.Version(
        hive_repo_dir.name, branch_name=args.dummy_bundle, commit_ish=args.commit
    )
    image_tag = args.image_tag_override or ver.commit.hexsha[0:10]
    validate_image(args.image_repo, image_tag, args.skip_image_validation)

    print("Checking out {}".format(ver.shortsha))
    try:
        ver.repo.git.checkout(ver.commit)
    except:
        print("Failed to checkout {}".format(ver.shortsha))
        raise

    if args.dummy_bundle:
        # Omit version graph stuff
        prev_version = None
    else:
        prev_version = get_previous_version(work_dir.name, CHANNEL_DEFAULT)
        os.chdir(hive_repo_dir.name)
        if ver.semver == prev_version:
            raise ValueError("Version {} already exists upstream".format(ver.semver))

    version_dir = generate_csv_base(
        bundle_dir.name, args.image_repo, ver, prev_version, image_tag
    )

    if args.dummy_bundle:
        copy_bundle(orig_wd, version_dir, ver)
    else:
        # redhat-openshift-ecosystem/community-operators-prod
        open_pr(
            work_dir.name,
            "git@github.com:%s/community-operators-prod.git" % args.github_user,
            "git@github.com:redhat-openshift-ecosystem/community-operators-prod.git",
            args.github_user,
            bundle_dir.name,
            ver,
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
            ver,
            args.hold,
            args.dry_run,
        )

    hive_repo_dir.cleanup()
    bundle_dir.cleanup()
    work_dir.cleanup()
