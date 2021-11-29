#!/bin/bash
set -e -o pipefail

# cd to the directory where this bash script is located at.
cd "$(dirname "$0")"
repo_root=$(dirname "$(pwd -P)")

docker build -t dockerized_tests dockerized_tests
docker run -e LOCAL_USER_ID="$(id -u)" \
  -v "${repo_root}":/repo:rw \
  --workdir=/repo \
  --entrypoint=/usr/local/bin/entrypoint.sh \
  -it \
  dockerized_tests \
  bash -c 'bazel \
      --bazelrc="/repo/improbable/bazelrc" \
      test \
      --config=imp-ci \
      --test_filter=-^TestEndpointSuccess$ \
      --test_filter=-^TestEndpointRegex$ \
      //prow/... \
      //ghproxy/...'
