#!/usr/bin/env bash
# Copyright 2021 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o nounset
set -o errexit
set -o pipefail

# cd to the repo root and setup go
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd -P)"
cd "${REPO_ROOT}"

PROJECT_ID="${PROJECT_ID:-}"
REGISTRY="${REGISTRY:-}"

# Image tag
IMAGE_TAG="${IMAGE_TAG:-}"
if [[ -z "${IMAGE_TAG}" ]]; then
  git_commit="$(git describe --tags --always --dirty)"
  build_date="$(date -u '+%Y%m%d')"
  IMAGE_TAG="v${build_date}-${git_commit}"
fi

LOCAL_BUILD="false"
IMAGE_NAME=""
SKIP_PUSH="false"

while [[ $# > 0 ]]; do
  case $1 in
    --local)
      LOCAL_BUILD="true";;
    --image)
      IMAGE_NAME="$2"
      shift;;
    --no-push)
      SKIP_PUSH="true";;
    *)
      echo "ERROR: unsupported args '$1'"
      echo "Only --local is supported."
      exit 1
  esac
  shift
done

# Default to run in Google Cloud Build
if [[ "$LOCAL_BUILD" == "false" ]]; then
  if [[ -z "${PROJECT_ID}" ]]; then
    echo "ERROR: PROJECT_ID must be set to the GCP project where google cloud build runs on."
    exit 1
  fi
  if [[ -z "${REGISTRY}" ]]; then
    echo "REGISTRY is not provided, defaults to PROJECT_ID: ${PROJECT_ID}."
    REGISTRY="${PROJECT_ID}"
  fi
  gcloud \
    builds \
    submit \
    --project=${PROJECT_ID} \
    --config=${REPO_ROOT}/images/prow-base/cloudbuild.yaml \
    --substitutions="_TAG=${IMAGE_TAG},_REPO=${REGISTRY}" \
    .
  exit 0
fi


# Building locally is way more complex than remote
# Running in local mode is meant to be one-off, must supply image name.
if [[ -z "${IMAGE_NAME}" ]]; then
  echo "ERROR: must provide --image"
  exit 1
fi
# REGISTRY needs to be supplied for local
if [[ -z "${REGISTRY}" ]]; then
  echo "ERROR: REGISTRY must be set"
  exit 1
fi

# Defaults
SOURCE_PATH="prow/cmd/$IMAGE_NAME"
declare -a PLATFORMS=(
  "amd64"
)
case $IMAGE_NAME in
  needs-rebase | cherrypicker | refresh)
    SOURCE_PATH="prow/external-plugins/$IMAGE_NAME";;
  ghproxy | label_sync)
    SOURCE_PATH="$IMAGE_NAME";;
  commenter | pr-creator | issue-creator)
    SOURCE_PATH="robots/$IMAGE_NAME";;
  configurator | transfigure)
    SOURCE_PATH="testgrid/cmd/$IMAGE_NAME";;
  gcsweb)
    SOURCE_PATH="gcsweb/cmd/$IMAGE_NAME";;
  bumpmonitoring)
    SOURCE_PATH="experiment/$IMAGE_NAME";;
  clonerefs | entrypoint | initupload | sidecar)
    echo "Multi-arch image"
    PLATFORMS+=("arm64" "s390x" "ppc64le");;
  *)
    ;; # Nothing to do
esac

echo "platforms are: ${PLATFORMS[@]}"

for platform in "${PLATFORMS[@]}"; do
  declare -a tags=(
    "${platform}"
    "latest-${platform}"
    "${IMAGE_TAG}-${platform}"
  )
  if [[ "$platform" == "amd64" ]]; then
    tags+=(
      "latest"
      "${IMAGE_TAG}"
    )
  fi
  tags_arg=""
  for tag in "${tags[@]}"; do
    tags_arg="${tags_arg} --tag=${REGISTRY}/${IMAGE_NAME}:${tag}"
  done

  # Docker build
  docker \
    build \
    $tags_arg \
    --build-arg=IMAGE_NAME=${IMAGE_NAME} \
    --build-arg=SOURCE_PATH=${SOURCE_PATH} \
    -f=./images/prow-base/Dockerfile_${platform} \
    --no-cache  \
    .

  if [[ "${SKIP_PUSH}" == "true" ]]; then
    echo "Skip pushing ${IMAGE_NAME}."
    continue
  fi

  for tag in "${tags[@]}"; do
    docker push ${REGISTRY}/${IMAGE_NAME}:${tag}
  done
done
