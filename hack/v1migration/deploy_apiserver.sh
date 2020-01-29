#!/bin/bash

set -e

oc patch hiveconfig hive --type='merge' -p $'spec:\n hiveAPIEnabled: true'