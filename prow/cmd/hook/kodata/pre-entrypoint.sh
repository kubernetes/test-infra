#!/bin/sh
#
# Launch custom (no k8s infra) hook build* as a GH Action to run a prow plugin
#
# DEBUG
set -x
set -e

env | grep INPUT_

if [ "${INPUT_PLUGIN}" != "" ]; then
    echo "${INPUT_PLUGIN}" > "/mnt/plugin"
fi

if [ "${INPUT_HMAC}" != "" ]; then
    echo "${INPUT_HMAC}" > "/mnt/hmac"
fi

if [ "${INPUT_GH-APP-ID}" != "" ]; then
    echo "${INPUT_GH-APP-ID}" > "/mnt/app_id"
fi

if [ "${INPUT_GH-APP-PK}" != "" ]; then
    echo "${INPUT_GH-APP-PK}" > "/mnt/app_pkey"
fi

# HBD? Would have to consider whether or not to expose this capability to
# Project Owners. For proof of concept demo purposes lets use ./config.yaml
# and ./plugins.yaml from this repo.
#
if [ "${INPUT_HOOK-CONFIG}" != "" ]; then
    echo "${INPUT_HOOK-CONFIG}" > /mnt/config.yaml
fi

if [ "${INPUT_PLUGIN_CONFIG}" != "" ]; then
    echo "${INPUT_PLUGIN-CONFIG}" > /mnt/plugins.yaml
fi

ls -la /mnt/
