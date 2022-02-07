#!/usr/bin/env bash
# Copyright 2022 The Kubernetes Authors.
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

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd $REPO_ROOT
source hack/build/setup-go.sh

readonly PROW_IMAGES_DEF_FILE="prow/.prow-images"
IMAGES=()
if [[ -n "${PROW_IMAGES:-}" ]]; then
  # Building prow images from supplied
  IFS=' ' read -ra IMAGES <<< "${PROW_IMAGES}"
else
  # Building all prow images
  while IFS= read -r image; do
    IMAGES+=($image)
  done < "$PROW_IMAGES_DEF_FILE"
fi

# overridable registry to use
KO_DOCKER_REPO="${KO_DOCKER_REPO:-}"
if [[ -z "${KO_DOCKER_REPO}" ]]; then
  echo "KO_DOCKER_REPO must be provided"
  exit 1
fi
export KO_DOCKER_REPO

# push or local tar?
PUSH="${PUSH:-false}"
# overridable auto-tag
TAG="${TAG:-"$(date +v%Y%m%d)-$(git describe --always --dirty)"}"

# build ko
cd 'hack/tools'
go build -o "${REPO_ROOT}/_bin/ko" github.com/google/ko
cd "${REPO_ROOT}"

echo "Images: ${IMAGES[@]}"

for image in "${IMAGES[@]}"; do
    echo "Building $image"
    name="$(basename "${image}")"
    # gather static files if there is any
    gather_static_file_script="${image}/gather-static.sh"
    if [[ -f $gather_static_file_script ]]; then
      source $gather_static_file_script
    fi
    # push or local tarball
    publish_args=(--tarball=_bin/"${name}".tar --push=false)
    if [[ "${PUSH}" != 'false' ]]; then
        publish_args=(--push=true)
    fi
    # specify tag
    publish_args+=(--base-import-paths --tags="${TAG}" --tags="latest" --tags="latest-root" --platform=linux/amd64)
    # actually build
    failed=0
    (set -x; _bin/ko publish "${publish_args[@]}" ./"${image}") || failed=1
    if [[ -f $gather_static_file_script ]]; then
      CLEAN=true $gather_static_file_script
    fi
    if (( failed )); then
      echo "Failed building image: ${image}"
      exit 1
    fi
done
