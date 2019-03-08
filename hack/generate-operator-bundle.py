#!/usr/bin/env python
#
# Generate an operator bundle for publishing to OLM. Copies appropriate files
# into a directory, and composes the ClusterServiceVersion which needs bits and
# pieces of our rbac and deployment files.
#
# Usage ./hack/generate-operator-bundle.py OUTPUT_DIR PREVIOUS_VERSION GIT_NUM_COMMITS GIT_HASH HIVE_IMAGE OLM_CHANNEL
#
# Commit count can be obtained with: git rev-list 9c56c62c6d0180c27e1cc9cf195f4bbfd7a617dd..HEAD --count
# This is the first hive commit, if we tag a release we can then switch to using that tag and bump the base version.

import datetime
import os
import sys
import yaml
import shutil

# This script will append the current number of commits given as an arg
# (presumably since some past base tag), and the git hash arg for a final
# version like: 0.1.189-3f73a592
VERSION_BASE = "0.1"

PACKAGE = '''packageName: hive-operator
channels:
- name: %s
  currentCSV: %s
'''


FAKE_PREVIOUS_CSV = '''apiVersion: operators.coreos.com/v1alpha1
kind: ClusterServiceVersion
metadata:
  name: %s
  namespace: placeholder
spec:
version: %s
'''

if len(sys.argv) != 7:
    print("USAGE: %s OUTPUT_DIR PREVIOUS_VERSION GIT_NUM_COMMITS GIT_HASH HIVE_IMAGE OLM_CHANNEL" % sys.argv[0])
    sys.exit(1)

outdir = sys.argv[1]
prev_version = sys.argv[2]
git_num_commits = sys.argv[3]
git_hash = sys.argv[4]
hive_image = sys.argv[5]
olm_channel = sys.argv[6]

full_version = "%s.%s-%s" % (VERSION_BASE, git_num_commits, git_hash)
print("Generating CSV for version: %s" % full_version)

if not os.path.exists(outdir):
    os.mkdir(outdir)

version_dir = os.path.join(outdir, full_version)
if not os.path.exists(version_dir):
    os.mkdir(version_dir)

# Copy all CSV files over to the bundle output dir:
crd_files = os.listdir('config/crds/')
for file_name in crd_files:
    full_path = os.path.join('config/crds', file_name)
    if (os.path.isfile(os.path.join('config/crds', file_name))):
        shutil.copy(full_path, os.path.join(version_dir, file_name))

with open('config/templates/hive-csv-template.yaml', 'r') as stream:
    csv = yaml.load(stream)

csv['spec']['install']['spec']['clusterPermissions'] = []

# Add our operator role to the CSV:
with open('config/operator/operator_role.yaml', 'r') as stream:
    operator_role = yaml.load(stream)
    csv['spec']['install']['spec']['clusterPermissions'].append(
        {
            'rules': operator_role['rules'],
            'serviceAccountName': 'hive-operator',
        })

# Add our hive-controllers role to the CSV:
with open('config/rbac/rbac_role.yaml', 'r') as stream:
    hive_role = yaml.load(stream)
    csv['spec']['install']['spec']['clusterPermissions'].append(
        {
            'rules': hive_role['rules'],
            'serviceAccountName': 'default',
        })

# Add our hiveadmission role to the CSV:
with open('config/hiveadmission/hiveadmission_rbac_role.yaml', 'r') as stream:
    hiveadmission_role = yaml.load(stream)
    csv['spec']['install']['spec']['clusterPermissions'].append(
        {
            'rules': hiveadmission_role['rules'],
            'serviceAccountName': 'hiveadmission',
        })

# Add our deployment spec for the hive operator:
with open('config/operator/operator_deployment.yaml', 'r') as stream:
    operator_components = []
    operator = yaml.load_all(stream)
    for doc in operator:
        operator_components.append(doc)
    operator_deployment = operator_components[1]
    csv['spec']['install']['spec']['deployments'][0]['spec'] = operator_deployment['spec']

# Update the deployment to use the defined image:
csv['spec']['install']['spec']['deployments'][0]['spec']['template']['spec']['containers'][0]['image'] = hive_image

# Update the versions to include git hash:
csv['metadata']['name'] = "hive-operator.v%s" % full_version
csv['spec']['version'] = full_version
csv['spec']['replaces'] = "hive-operator.v%s" % prev_version

# Set the CSV createdAt annotation:
now = datetime.datetime.now()
csv['metadata']['annotations']['createdAt'] = now.strftime("%Y-%m-%dT%H:%M:%SZ")

# Write the CSV to disk:
csv_filename = "hive-operator.v%s.clusterserviceversion.yaml" % full_version
csv_file = os.path.join(version_dir, csv_filename)
with open(csv_file, 'w') as outfile:
    yaml.dump(csv, outfile, default_flow_style=False)
print("Wrote ClusterServiceVersion: %s" % csv_file)

# Write the package file:
package_file = os.path.join(outdir, 'hive.package.yaml')
with open(package_file, 'w') as outfile:
    outfile.write(PACKAGE % (olm_channel, csv['metadata']['name']))
print("Wrote package file: %s" % package_file)



# Write a fake previous CSV:
# TODO: we're not 100% sure this should be required, but it seems to be.
fake_prev_version_dir = os.path.join(outdir, prev_version)
if not os.path.exists(fake_prev_version_dir):
    os.mkdir(fake_prev_version_dir)
fake_prev_csv_file = os.path.join(fake_prev_version_dir, "hive-operator.v%s.clusterserviceversion.yaml" % prev_version)
with open(fake_prev_csv_file, 'w') as outfile:
    outfile.write(FAKE_PREVIOUS_CSV % (csv['spec']['replaces'], prev_version))
print("Wrote fake previous ClusterServiceVersion file: %s" % fake_prev_csv_file)
