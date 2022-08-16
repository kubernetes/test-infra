#!/bin/sh
#
# Launch custom (no k8s infra) hook build* as a GH Action to run a prow plugin
#
# DEBUG
set -x
set -e

if [ "${INPUT_HMAC}" != "" ]; then
    echo "${INPUT_HMAC}" > "$HOME/hmac"
fi

if [ "${INPUT_GH_APP_ID}" != "" ]; then
    echo "${INPUT_GH-APP-ID}" > "$HOME/app_id"
fi

if [ "${INPUT_GH_APP_PK}" != "" ]; then
    echo "${INPUT_GH-APP-PK}" > "$HOME/app_pkey"
fi

# HBD? Would have to consider whether or not to expose this capability to
# Project Owners. For proof of concept demo purposes lets use ./config.yaml
# and ./plugins.yaml from this repo.
#
if [ "${INPUT_HOOK-CONFIG}" != "" ]; then
    echo "${INPUT_HOOK-CONFIG}" > $HOME/config.yaml
fi

if [ "${INPUT_PLUGIN_CONFIG}" != "" ]; then
    echo "${INPUT_PLUGIN-CONFIG}" > $HOME/plugins.yaml
fi

ls -la $HOME/
