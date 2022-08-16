#!/bin/sh
#
# Launch custom (no k8s infra) hook build* as a GH Action to run a prow plugin
#
# DEBUG
set -x
set -e

env | grep INPUT_

PROW_CONFIGFILE=$HOME/config.yaml
PLUGIN_CONFIGFILE=$HOME/plugins.yaml
HMAC_FILE=$HOME/hmac
GITHUB_TOKENFILE=$HOME/github_token

if [ "${INPUT_HOOK-CONFIG}" != "" ]; then
    echo "${INPUT_HOOK-CONFIG}" > "${PROW_CONFIGFILE}"
else
    cp "/var/run/ko/config.yaml" "${PROW_CONFIGFILE}"
fi

if [ "${INPUT_PLUGIN_CONFIG}" != "" ]; then
    echo "${INPUT_PLUGIN-CONFIG}" > "${PLUGIN_CONFIGFILE}"
else
    cp "/var/run/ko/plugins.yaml" "${PLUGIN_CONFIGFILE}"
fi

echo "${GITHUB_TOKEN}" > "${GITHUB_TOKENFILE}"

/ko-app/hook \
    --config-path "${PROW_CONFIGFILE}" \
    --plugin-config "${PLUGIN_CONFIGFILE}" \
    --hmac-secret-file "${HMAC_FILE}" \
    --github-token-path "${GITHUB_TOKENFILE}" \
    --dry-run=false
