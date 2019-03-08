#!/usr/bin/env python
#
# Generate an operator bundle for publishing to OLM. Copies appropriate files
# into a directory, and composes the ClusterServiceVersion which needs bits and
# pieces of our rbac and deployment files.
#
# Usage ./hack/generate-operator-bundle.py OUTPUT_DIR GIT_HASH

import datetime
import os
import sys
import yaml
import shutil

PACKAGE = '''packageName: hive-operator
channels:
- name: alpha
  currentCSV: %s
'''

if len(sys.argv) != 4:
    print("USAGE: %s OUTPUT_DIR GIT_HASH HIVE_IMAGE" % sys.argv[0])
    sys.exit(1)

outdir = sys.argv[1]
git_hash = sys.argv[2]
hive_image = sys.argv[3]

if not os.path.exists(outdir):
    os.mkdir(outdir)

# Copy all CSV files over to the bundle output dir:
crd_files = os.listdir('config/crds/')
for file_name in crd_files:
    full_path = os.path.join('config/crds', file_name)
    if (os.path.isfile(os.path.join('config/crds', file_name))):
        shutil.copy(full_path, os.path.join(outdir, file_name))

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
csv['metadata']['name'] = "hive-operator-%s" % git_hash
csv['spec']['version'] = git_hash

# Set the CSV createdAt annotation:
now = datetime.datetime.now()
csv['metadata']['annotations']['createdAt'] = now.strftime("%Y-%m-%dT%H:%M:%SZ")


# Write the CSV to disk:
csv_file = os.path.join(outdir, 'hive-csv.yaml')
with open(csv_file, 'w') as outfile:
    yaml.dump(csv, outfile, default_flow_style=False)

# Write the package file:
package_file = os.path.join(outdir, 'hive-package.yaml')
with open(package_file, 'w') as outfile:
    outfile.write(PACKAGE % csv['metadata']['name'])
