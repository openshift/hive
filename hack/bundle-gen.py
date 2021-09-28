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

# HIVE_VERSION_PREFIX is the prefix of the Hive version that will be constructed by this script
# the version used for images and bundles will be "{prefix}.{number of commits}-{git hash}" eg. "v1.2.3187-18827f6"
HIVE_VERSION_PREFIX = "v1.2"

HIVE_REPO = "git@github.com:openshift/hive.git"

# Hive dir within both:
# https://github.com/redhat-openshift-ecosystem/community-operators-prod
# https://github.com/k8s-operatorhub/community-operators
HIVE_SUB_DIR = 'operators/hive-operator'

OPERATORHUB_HIVE_IMAGE_DEFAULT = 'quay.io/openshift-hive/hive'

REGISTRY_AUTH_FILE_DEFAULT = '{}/.docker/config.json'.format(os.environ['HOME'])

COMMUNITY_OPERATORS_HIVE_PKG_URL = 'https://raw.githubusercontent.com/redhat-openshift-ecosystem/community-operators-prod/main/operators/hive-operator/hive.package.yaml'

CHANNEL_DEFAULT = 'alpha'
BRANCH_DEFAULT = 'master'

SUBPROCESS_REDIRECT = subprocess.DEVNULL


def get_params():
    parser = argparse.ArgumentParser(description='Hive Bundle Generator and Publishing Script')
    parser.add_argument('--verbose',
                        default=False,
                        help='Show more details while running',
                        action='store_true')
    parser.add_argument('--dry-run',
                        default=False,
                        help='Test run that skips pushing branches and submitting PRs',
                        action='store_true')
    parser.add_argument('--branch',
                        default=False,
                        help='''The branch (or commit-ish) from which to build and push the image.
                                If unspecified, we assume `master`. If we're using `master`, we
                                generate the bundle version number based on the hive version prefix `{}` and
                                update the `alpha` channel. If BRANCH *also* corresponds to an `ocm-X.Y`,
                                we'll also update that channel with the same bundle.If BRANCH corresponds
                                to an `ocm-X.Y` but *not* `master`, we'll generate the bundle version
                                number based on `X.Y` and update only that channel.'''.format(HIVE_VERSION_PREFIX))
    parser.add_argument('--registry-auth-file',
                        default=REGISTRY_AUTH_FILE_DEFAULT,
                        help='Path to registry auth file')
    parser.add_argument('--github-user',
                        default=os.environ['USER'],
                        help="User's github username, if different than $USER")
    parser.add_argument('--hold',
                        default=False,
                        help='Adds a /hold comment in commit body to prevent the PR from merging (use "/hold cancel" to remove)',
                        action='store_true')
    args = parser.parse_args()

    if args.branch and args.branch == BRANCH_DEFAULT:
        parser.error('--branch=master is assumed by this script and corresponds to the alpha channel')

    if not os.path.isfile(args.registry_auth_file):
        parser.error('--registry-auth-file ({}) does not exist, provide --registry-auth-file'.format(args.registry_auth_file))

    if args.verbose:
        global SUBPROCESS_REDIRECT
        SUBPROCESS_REDIRECT = None

    return args

# build_and_push_image uses buildah to build the HIVE image (tagged with image_tag) and then pushes the image to quay
def build_and_push_image(repo_dir, registry_auth_file, image_tag, branch, dry_run):
    repo = git.Repo(repo_dir)

    print('Checkout out {} branch'.format(branch))
    try:
        repo.git.checkout(branch)
    except:
        print('Failed to checkout branch {}'.format(branch))
        raise

    container_name = '{}:{}'.format(OPERATORHUB_HIVE_IMAGE_DEFAULT, image_tag)

    if dry_run:
        print('Skipping build of container {} due to dry-run'.format(container_name))
        return

    # Did we already build it locally?
    cp = subprocess.run('buildah images -nq {}'.format(container_name).split(), stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    if cp.returncode == 0:
        print('Container {} already exists locally; not rebuilding.'.format(container_name))
    else:
        # build/push the thing
        print('Building container {}'.format(container_name))

        cmd = 'buildah bud --tag {} -f ./Dockerfile'.format(container_name).split()
        subprocess.run(cmd)

    print("Pushing container")
    cmd = 'buildah push '
    if registry_auth_file != None:
        cmd = cmd + ' --authfile={} '.format(registry_auth_file)
    cmd = cmd + ' {}'.format(container_name)
    subprocess.run(cmd.split())

# gen_hive_version generates and returns the hive version eg. "v1.2.3187-18827f6"
def gen_hive_version(repo_dir, branch, commit_hash, prefix):
    repo = git.Repo(repo_dir)

    print('Checkout out {} branch'.format(branch))
    try:
        repo.git.checkout(branch)
    except:
        print('Failed to checkout branch {}'.format(branch))
        raise

    # sha is the first 7 characters of commit_hash
    sha = str(repo.rev_parse(commit_hash))[0:7]

    # num commits is the number of git commits counted from the first commit to the provided commit_hash
    # this is the equivalent of running:
    # git rev-list `git rev-list --parents HEAD | egrep "^[a-f0-9]{40}$"`..HEAD --count
    parent = repo.git.rev_list('--max-parents=0', commit_hash)
    num_commits = repo.git.rev_list('--count', '{parent}..{commit_hash}'.format(parent=parent, commit_hash=commit_hash))

    return '{prefix}.{commits}-{sha}'.format(prefix=prefix, commits=num_commits, sha=sha)

# get_previous_version grabs the previous hive version from COMMUNITY_OPERATORS_HIVE_PKG_URL package yaml
# for the provided channel_name.
def get_previous_version(channel_name):
    http = urllib3.PoolManager()
    r = http.request('GET', COMMUNITY_OPERATORS_HIVE_PKG_URL)
    pkg_yaml = yaml.load(r.data.decode('utf-8'), Loader=yaml.FullLoader)
    try:
        for channel in pkg_yaml['channels']:
            if channel['name'] == channel_name:
                return channel['currentCSV'].replace('hive-operator.', '')
    except:
        print('Unable to determine previous hive version from {}', COMMUNITY_OPERATORS_HIVE_PKG_URL)
        raise

def generate_csv_base(bundle_dir, version, prev_version, channel):
    print("Writing bundle files to directory: %s" % bundle_dir)
    print("Generating CSV for version: %s" % version)

    crds_dir = 'config/crds'
    csv_template = 'config/templates/hive-csv-template.yaml'
    operator_role = 'config/operator/operator_role.yaml'
    deployment_spec = 'config/operator/operator_deployment.yaml'
    package_file = os.path.join(bundle_dir, 'hive.package.yaml')

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
            with open(dest_path, 'r') as stream:
                crd_csv = yaml.load(stream, Loader=yaml.SafeLoader)
                owned_crds.append(
                        {
                            'description': crd_csv['spec']['versions'][0]['schema']['openAPIV3Schema']['description'],
                            'displayName': crd_csv['spec']['names']['kind'],
                            'kind': crd_csv['spec']['names']['kind'],
                            'name': crd_csv['metadata']['name'],
                            'version': crd_csv['spec']['versions'][0]['name'],
                        })

    with open(csv_template, 'r') as stream:
        csv = yaml.load(stream, Loader=yaml.SafeLoader)

    csv['spec']['customresourcedefinitions']['owned'] = owned_crds

    csv['spec']['install']['spec']['clusterPermissions'] = []

    # Add our operator role to the CSV:
    with open(operator_role, 'r') as stream:
        operator_role = yaml.load(stream, Loader=yaml.SafeLoader)
        csv['spec']['install']['spec']['clusterPermissions'].append(
            {
                'rules': operator_role['rules'],
                'serviceAccountName': 'hive-operator',
            })

    # Add our deployment spec for the hive operator:
    with open(deployment_spec, 'r') as stream:
        operator_components = []
        operator = yaml.load_all(stream, Loader=yaml.SafeLoader)
        for doc in operator:
            operator_components.append(doc)
        operator_deployment = operator_components[1]
        csv['spec']['install']['spec']['deployments'][0]['spec'] = operator_deployment['spec']

    # Update the versions to include git hash:
    csv['metadata']['name'] = "hive-operator.%s" % version
    csv['spec']['version'] = version
    csv['spec']['replaces'] = "hive-operator.%s" % prev_version

    # Update the deployment to use the defined image:
    image_ref = "%s:%s" % (OPERATORHUB_HIVE_IMAGE_DEFAULT, version)
    csv['spec']['install']['spec']['deployments'][0]['spec']['template']['spec']['containers'][0]['image'] = image_ref
    csv['metadata']['annotations']['containerImage'] = image_ref

    # Set the CSV createdAt annotation:
    now = datetime.datetime.now()
    csv['metadata']['annotations']['createdAt'] = now.strftime("%Y-%m-%dT%H:%M:%SZ")

    # Write the CSV to disk:
    csv_filename = "hive-operator.%s.clusterserviceversion.yaml" % version
    csv_file = os.path.join(version_dir, csv_filename)
    with open(csv_file, 'w') as outfile:
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
    name = "hive-operator.%s" % version
    document = document_template % (name, channel, channel)

    with open(package_file, 'w') as outfile:
        yaml.dump(yaml.load(document, Loader=yaml.SafeLoader), outfile, default_flow_style=False)
    print("Wrote package: %s" % package_file)

def open_pr(work_dir, fork_repo, upstream_repo, gh_username, bundle_source_dir, new_version, update_channels, hold, dry_run):

    dir_name = fork_repo.split('/')[1][:-4]

    dest_github_org = upstream_repo.split(':')[1].split('/')[0]
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
        print('Failed to clone repo {} to {}'.format(fork_repo, repo_full_path))
        raise

    # get to the right place on the filesystem
    print("Working in %s" % repo_full_path)
    os.chdir(repo_full_path)

    repo = git.Repo(repo_full_path)
    try:
        repo.create_remote('upstream', upstream_repo)
    except:
        print('Failed to create upstream remote')
        raise

    print("Fetching latest upstream")
    try:
        repo.remotes.upstream.fetch()
    except:
        print('Failed to fetch upstream')
        raise

    # Starting branch
    print("Checkout latest upstream/main")
    try:
        repo.git.checkout('upstream/main')
    except:
        print('Failed to checkout upstream/main')
        raise

    branch_name = 'update-hive-{}'.format(new_version)
    pr_title = "Update Hive community operator to {}".format(new_version)
    print("Starting {}".format(pr_title))

    print("Create branch {}".format(branch_name))
    try:
        repo.git.checkout('-b', branch_name)
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
    bundle_manifests_file = os.path.join(repo_full_path, HIVE_SUB_DIR, "hive.package.yaml")
    bundle = {}
    with open(bundle_manifests_file, 'r') as a_file:
        bundle = yaml.load(a_file, Loader=yaml.SafeLoader)

    found = False
    for channel in bundle["channels"]:
        if channel["name"] in update_channels:
            found = True
            channel["currentCSV"] = "hive-operator.{}".format(new_version)

    if not found:
        print("did not find a CSV channel to update")
        sys.exit(1)

    with open(bundle_manifests_file, 'w') as outfile:
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
        repo.git.commit('--signoff', '--message={}'.format(pr_title))
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
        if resp.status_code != 201: #201 == Created
            print(resp.text)
            sys.exit(1)

        json_content = json.loads(resp.content.decode('utf-8'))
        print("PR opened: {}".format(json_content["html_url"]))

    else:
        print("Skipping branch push due to dry-run")
    print()

def branch_is_on_master_commit(branch, repo_dir):
    repo = git.Repo(repo_dir)

    print('Checkout out {} branch'.format(branch))
    try:
        repo.git.checkout(branch)
    except:
        print('Failed to checkout branch {}'.format(branch))
        raise

    return repo.rev_parse('master') == repo.rev_parse(branch)

def buildah_installed():
    return shutil.which('buildah') is not None

if __name__ == "__main__":
    args = get_params()

    if not buildah_installed():
        print('buildah not found, please install buildah')
        sys.exit(1)

    hive_repo_dir = tempfile.TemporaryDirectory(prefix='hive-repo-')

    print('Cloning {} to {}'.format(HIVE_REPO, hive_repo_dir.name))
    try:
        git.Repo.clone_from(HIVE_REPO, hive_repo_dir.name)
    except:
        print('Failed to clone repo {} to {}'.format(HIVE_REPO, hive_repo_dir.name))
        raise

    update_channels = [CHANNEL_DEFAULT]
    if args.branch:
        if branch_is_on_master_commit(args.branch, hive_repo_dir.name):
            update_channels.append(args.branch)
        else:
            update_channels = [args.branch]

    hive_version = ""
    bundle_dir = tempfile.TemporaryDirectory(prefix='hive-operator-bundle-')
    work_dir = tempfile.TemporaryDirectory(prefix='operatorhub-push-')

    if CHANNEL_DEFAULT in update_channels:
        hive_version = gen_hive_version(hive_repo_dir.name, BRANCH_DEFAULT, 'HEAD', HIVE_VERSION_PREFIX)
        build_and_push_image(hive_repo_dir.name, args.registry_auth_file, hive_version, BRANCH_DEFAULT, args.dry_run)
        previous_hive_version = get_previous_version(CHANNEL_DEFAULT)
        generate_csv_base(bundle_dir.name, hive_version, previous_hive_version, CHANNEL_DEFAULT)
    else:
        # the version prefix is determined by the branch name when the top commit of branch
        # doesn't correspond with HEAD of the master branch
        # eg. args.branch = "ocm-2.4" -> hive_version_prefix = "v2.4"
        hive_version_prefix = 'v{}'.format(args.branch.split('-', 1)[1])
        hive_version = gen_hive_version(hive_repo_dir.name, args.branch, 'HEAD', hive_version_prefix)
        build_and_push_image(hive_repo_dir.name, args.registry_auth_file, hive_version, args.branch, args.dry_run)
        previous_hive_version = get_previous_version(args.branch)
        generate_csv_base(bundle_dir.name, hive_version, previous_hive_version, args.branch)

    # redhat-openshift-ecosystem/community-operators-prod
    open_pr(work_dir.name,
        "git@github.com:%s/community-operators-prod.git" % args.github_user,
        "git@github.com:redhat-openshift-ecosystem/community-operators-prod.git",
        args.github_user, bundle_dir.name, hive_version, update_channels, args.hold, args.dry_run)
    # k8s-operatorhub/community-operators
    open_pr(work_dir.name,
         "git@github.com:%s/community-operators.git" % args.github_user,
         "git@github.com:k8s-operatorhub/community-operators.git",
         args.github_user, bundle_dir.name, hive_version, update_channels, args.hold, args.dry_run)

    hive_repo_dir.cleanup()
    bundle_dir.cleanup()
    work_dir.cleanup()