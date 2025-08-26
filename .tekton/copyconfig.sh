#!/usr/bin/env bash

set -e

if [ -z "$1" ]; then
    echo "Missing base version (e.g. 28, 29, 210)"
    exit 1
fi

if [ "$1" == "26" ]; then
    echo "mce-26 is a snowflake version, please use another base version"
    exit 1
fi

BASE_PUSH_FILE="hive-mce-$1-push.yaml"
BASE_PULL_FILE="hive-mce-$1-pull-request.yaml"

if [ ! -f "$BASE_PUSH_FILE" ]; then
    echo "$BASE_PUSH_FILE does not exist"
    exit 1
fi

if [ ! -f "$BASE_PULL_FILE" ]; then
    echo "$BASE_PULL_FILE does not exist"
    exit 1
fi

if [ -z "$2" ]; then
    echo "Missing new version (e.g. 211, 212, 213)"
    exit 1
fi

NEW_PUSH_FILE="hive-mce-$2-push.yaml"
NEW_PULL_FILE="hive-mce-$2-pull-request.yaml"


if [ -f "$NEW_PUSH_FILE" ]; then
    echo "$NEW_PUSH_FILE already exists"
    exit 1
fi


if [ -f "$NEW_PULL_FILE" ]; then
    echo "$NEW_PULL_FILE already exists"
    exit 1
fi

cp $BASE_PUSH_FILE $NEW_PUSH_FILE
cp $BASE_PULL_FILE $NEW_PULL_FILE

sed -i "s/mce-$1/mce-$2/g" $NEW_PUSH_FILE $NEW_PULL_FILE
