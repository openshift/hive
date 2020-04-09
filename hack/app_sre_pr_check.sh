#!/bin/bash

# AppSRE team CD

set -exv

BASE_IMG="hive"
IMG="${BASE_IMG}:latest"

BUILD_CMD="docker build" IMG="$IMG" make GO_REQUIRED_MIN_VERSION:= docker-build
