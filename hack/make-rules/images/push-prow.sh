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
IMAGE_PATH="${IMAGE_PATH:-}"

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

# Running in local mode is meant to be one-off, must supply image name.
if [[ "$LOCAL_BUILD" == "true" ]]; then
  if [[ -z "${IMAGE_NAME}" ]]; then
    echo "ERROR: must provide --image"
    exit 1
  fi
  if [[ -z "${REGISTRY}" ]]; then
    echo "ERROR: REGISTRY must be set"
    exit 1
  fi
  if [[ -n "${IMAGE_PATH}" ]]; then
    IMAGE_NAME=${IMAGE_PATH}/${IMAGE_NAME}
  fi
  docker build -t ${REGISTRY}/${IMAGE_NAME}:${IMAGE_TAG} --build-arg IMAGE_NAME=${IMAGE_NAME} -f ./images/prow-base/Dockerfile --no-cache  .
  if [[ "${SKIP_PUSH}" != "true" ]]; then
    docker push ${REGISTRY}/${IMAGE_NAME}:${IMAGE_TAG}
  fi
  exit 0
fi

# Now running Google Cloud Build
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
