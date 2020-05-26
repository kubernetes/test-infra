#!/usr/bin/env bash

###
# Builds Hook and pushes the container to the Improbable GCR
###

export PROW_REPO_OVERRIDE="eu.gcr.io/windy-oxide-102215"
export DOCKER_REPO_OVERRIDE="${PROW_REPO_OVERRIDE}"
export EDGE_PROW_REPO_OVERRIDE="${PROW_REPO_OVERRIDE}"

# By default, try to use Bazelisk, since this repo doesn't have `tools/bazel` set up.
# If that fails, try just `bazel`, and if that fails, just bail out.
BAZEL_BIN="bazelisk"
command -v "${BAZEL_BIN}" >"/dev/null"
if [[ $? != 0 ]]; then
  BAZEL_BIN="bazel"
  echo "WARNING: Bazelisk not found, attempting to use 'bazel' directly."
  echo "Without bazelisk installed, you may need to manually install the correct Bazel version."
  command -v "${BAZEL_BIN}" >"/dev/null"
  if [[ $? != 0 ]]; then
    echo "Bazel not found - install it to build this tool."
    exit 1
  fi
fi

echo "Using ${BAZEL_BIN} as Bazel."

"${BAZEL_BIN}" \
  --bazelrc="$(dirname "$0")/bazelrc" \
  run \
  --config=imp-release \
  //improbable:improbable-push
