#!/usr/bin/env bash

###
# Builds Hook and pushes the container to the Improbable GCR
###

export PROW_REPO_OVERRIDE="eu.gcr.io/windy-oxide-102215"
export DOCKER_REPO_OVERRIDE="${PROW_REPO_OVERRIDE}"
export EDGE_PROW_REPO_OVERRIDE="${PROW_REPO_OVERRIDE}"

# By default, try to use Bazelisk, since this repo doesn't have `tools/bazel` set up.
# If that fails, try just `bazel`, and if that fails, just bail out.
echo "Attempting to run script with Bazelisk first."
BAZEL_BIN="bazelisk"
command -v "${BAZEL_BIN}" >"/dev/null"
if [[ $? != 0 ]]; then
  BAZEL_BIN="bazel"
  echo "Bazelisk not found, attempting to use 'bazel' directly"
  command -v "${BAZEL_BIN}" >"/dev/null" || (echo "Bazel not found - install it to build this tool." && exit 1)
fi

echo "BAZEL_BIN is set to ${BAZEL_BIN}"


"${BAZEL_BIN}" run \
  --platforms=@io_bazel_rules_go//go/toolchain:linux_amd64 \
  //prow:improbable-push
