#!/usr/bin/env python
#
# Generate an operator bundle for publishing to OLM. Copies appropriate files
# into a directory, and composes the ClusterServiceVersion which needs bits and
# pieces of our rbac and deployment files.
#
# Usage: hack/generate-operator-bundle.py (osd|operatorhub)
#
#  See help for options.
#

import argparse
import datetime
import os
import shutil
import subprocess
import tempfile
import yaml


OSD_HIVE_IMAGE_DEFAULT = 'quay.io/app-sre/hive'
OPERATORHUB_HIVE_IMAGE_DEFAULT = 'quay.io/openshift-hive/hive'
CHANNEL_DEFAULT = 'alpha'


def current_operatorhub_version():
    result = subprocess.check_output("git describe --abbrev=0", shell=True).strip()
    # remove the v -- OLM bundles need to start with the digit
    if result[0] == 'v':
        result = result[1:]
    return result


def generate_operatorhub_version(prev_version=current_operatorhub_version()):
    # bump the zstream
    previous_z = int(prev_version.split('.')[-1])
    new_z = previous_z + 1
    new_version = "%s.%s" % ('.'.join(prev_version.split('.')[:-1]), new_z)
    return new_version


def generate_osd_version():
    result = subprocess.check_output("git describe --abbrev=7", shell=True).strip()
    # remove the v -- OLM bundles need to start with the digit
    if result[0] == 'v':
        result = result[1:]
    return result


def generate_csv_operatorhub():
    print("Generating CSV for operatorhub")
    prev_version = args.prev_version
    if not prev_version:
        prev_version = current_operatorhub_version()
    version = args.new_version
    if not version:
        version = generate_operatorhub_version(prev_version)
    if version == prev_version:
        raise ValueError("version and prev_version cannot be the same: %s" % version)
    generate_csv_base(version, prev_version, args.hive_image)


def generate_csv_osd():
    print("Generating CSV for OpenShift Dedicated")
    if not args.prev_version:
        raise ValueError("--previous-version must be specified")
    version = generate_osd_version()
    generate_csv_base(version, args.prev_version, args.hive_image)


def generate_csv_base(version, prev_version, hive_image):

    bundle_dir = args.bundle_dir
    if not bundle_dir:
        bundle_dir = tempfile.mkdtemp()

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

    # Copy all CSV files over to the bundle output dir:
    crd_files = os.listdir(crds_dir)
    for file_name in crd_files:
        full_path = os.path.join(crds_dir, file_name)
        if os.path.isfile(os.path.join(crds_dir, file_name)):
            shutil.copy(full_path, os.path.join(version_dir, file_name))

    with open(csv_template, 'r') as stream:
        csv = yaml.load(stream, Loader=yaml.SafeLoader)

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
    csv['metadata']['name'] = "hive-operator.v%s" % version
    csv['spec']['version'] = version
    csv['spec']['replaces'] = "hive-operator.v%s" % prev_version

    # Update the deployment to use the defined image:
    image_ref = hive_image
    if not ":" in image_ref:
        image_ref = "%s:v%s" % (hive_image, version)
    csv['spec']['install']['spec']['deployments'][0]['spec']['template']['spec']['containers'][0]['image'] = image_ref
    csv['metadata']['annotations']['containerImage'] = image_ref

    # Set the CSV createdAt annotation:
    now = datetime.datetime.now()
    csv['metadata']['annotations']['createdAt'] = now.strftime("%Y-%m-%dT%H:%M:%SZ")

    # Write the CSV to disk:
    csv_filename = "hive-operator.v%s.clusterserviceversion.yaml" % version
    csv_file = os.path.join(version_dir, csv_filename)
    with open(csv_file, 'w') as outfile:
        yaml.dump(csv, outfile, default_flow_style=False)
    print("Wrote ClusterServiceVersion: %s" % csv_file)

    # generate package
    generate_package(package_file, args.channel, version)

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

    with open(package_file, 'w') as outfile:
        yaml.dump(yaml.load(document, Loader=yaml.SafeLoader), outfile, default_flow_style=False)
    print("Wrote package: %s" % package_file)


parser = argparse.ArgumentParser(
    prog='OpenShift Hive Operator CSV generator',
    description='created CSV and related artifacts for operatorhub.io and OpenShift Dedicated.',
    formatter_class=argparse.RawDescriptionHelpFormatter,
)

subparsers = parser.add_subparsers()

osd_parser = subparsers.add_parser('osd', help='Generate CSV for OpenShift Dedicated deployment.')
osd_parser.add_argument('--previous-version', dest='prev_version', metavar='VERSION', help='Previous version, the version being replaced')
osd_parser.add_argument('--hive-image', dest='hive_image', metavar='IMAGE', help='The hive image to deploy', default=OSD_HIVE_IMAGE_DEFAULT)
osd_parser.add_argument('--channel', dest='channel', metavar='CHANNEL', help='The operator channel to use', default=CHANNEL_DEFAULT)
osd_parser.add_argument('--bundle-dir', dest='bundle_dir', metavar='BUNDLE_DIR', help='The bundle directory to write to', default=None)
osd_parser.set_defaults(func=generate_csv_osd)

operatorhub_parser = subparsers.add_parser('operatorhub', help='Generate CSV for operatorhub deployment.')
operatorhub_parser.add_argument('--previous-version', dest='prev_version', metavar='VERSION', help='Previous version, the version being replaced', default=None)
operatorhub_parser.add_argument('--new-version', dest='new_version', metavar='VERSION', help='The new version', default=None)
operatorhub_parser.add_argument('--hive-image', dest='hive_image', metavar='IMAGE', help='The hive image to deploy', default=OPERATORHUB_HIVE_IMAGE_DEFAULT)
operatorhub_parser.add_argument('--channel', dest='channel', metavar='CHANNEL', help='The operator channel to use', default=CHANNEL_DEFAULT)
operatorhub_parser.add_argument('--bundle-dir', dest='bundle_dir', metavar='BUNDLE_DIR', help='The bundle directory to write to', default=None)
operatorhub_parser.set_defaults(func=generate_csv_operatorhub)

args = parser.parse_args()
args.func()
