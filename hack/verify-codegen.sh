#!/bin/bash

SCRIPT_ROOT=$(dirname ${BASH_SOURCE})/..
VERIFY=--verify-only ${SCRIPT_ROOT}/hack/update-codegen.sh