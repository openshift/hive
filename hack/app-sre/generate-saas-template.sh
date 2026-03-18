#!/usr/bin/env bash

set -e

# Use YQ environment variable if set, otherwise default to '$YQ' in PATH
YQ=${YQ:-$YQ}

usage() {
    cat <<EOF
Usage: $0 saas_template_stub saas_object_file out_file

Collate \`saas_object_file\` into the objects list in \`saas_template_stub\` and
write the result to \`out_file\`.

Parameters:
  saas_template_stub: A yaml file defining a Template with an empty \`objects\`
        list.
  saas_object_file: A yaml file containing some number of yaml documents
        defining kube objects to be injected into the \`saas_template_stub\`.
  out_file: Path to which to write the resulting yaml file. If it exists, it
        will be overwritten.
EOF
}

# Check for help flag or incorrect number of arguments
if [ $# -ne 3 ]; then
    usage
    exit 1
fi

for arg in "$@"; do
    if [[ "$arg" == *"-h"* ]]; then
        usage
        exit 1
    fi
done

saas_template_stub="$1"
saas_object_file="$2"
out_file="$3"

# Validate input files exist
for f in "$saas_template_stub" "$saas_object_file"; do
    if [ ! -f "$f" ]; then
        echo "$f: No such file" >&2
        exit 1
    fi
done

# Check if out_file exists and is not a regular file
if [ -e "$out_file" ] && [ ! -f "$out_file" ]; then
    echo "$out_file: Not a file" >&2
    exit 1
fi

# Count and load objects from the saas_object_file
# $YQ splits multi-document YAML into separate documents
object_count=$($YQ ea '[.] | length' "$saas_object_file")
echo "Loaded $object_count objects from $saas_object_file"

# Determine output message
if [ -e "$out_file" ]; then
    echo "Overwriting existing output file $out_file"
else
    echo "Writing output file $out_file"
fi

# Merge the template stub with objects from saas_object_file
# 1. Read all documents from saas_object_file and collect into array
# 2. Read template stub
# 3. Set template.objects to the collected array
$YQ ea '
    select(fileIndex == 0) as $template |
    [select(fileIndex == 1)] as $objects |
    $template | .objects = $objects
' "$saas_template_stub" "$saas_object_file" > "$out_file"
