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

readonly DEFAULT_ARCH="linux/amd64"

GIT_TAG="$(date +v%Y%m%d)-$(git describe --always --dirty)"
TAG_SET=(
  "latest"
  "latest-root"
  "${GIT_TAG}"
)
ALL_ARCHES=("arm64" "s390x" "ppc64le")

# takes arch as string, returns space separated list of tags basesd on arch
tags-arg() {
  local tags=(${TAG_SET[@]})
  local arches="$1"
  if [[ "${arches}" == "all" ]]; then
    for arch in "${ALL_ARCHES[@]}"; do
      tags+=("${arch}")
      for base in "${TAG_SET[@]}"; do
        tags+=("${base}-${arch}")
      done
    done
  fi

  for tag in "${tags[@]}"; do
    echo "--tags=${tag}"
  done
}

# overridable IMAGES def file
PROW_IMAGES_DEF_FILE="${PROW_IMAGES_DEF_FILE:-prow/.prow-images}"
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

# push or local tar?
PUSH="${PUSH:-false}"
# overridable registry to use
KO_DOCKER_REPO="${KO_DOCKER_REPO:-}"
if [[ -z "${KO_DOCKER_REPO}" ]]; then
  if [[ "${PUSH}" != "false" ]]; then
    echo "KO_DOCKER_REPO must be provided when PUSH is true"
    exit 1
  fi
  # KO_DOCKER_REPO is not important for local build
  KO_DOCKER_REPO="do.not/matter/at/all"
fi
export KO_DOCKER_REPO

# build ko
cd 'hack/tools'
go build -o "${REPO_ROOT}/_bin/ko" github.com/google/ko
cd "${REPO_ROOT}"

echo "Images: ${IMAGES[@]}"

# Helper function
gather-static-script() {
  local image="$1"
  if [[ "$image" == "" || "$image" =~ \#.* ]]; then
    return "__PATH_NOT_EXIST__"
  fi
  read image_dir arch < <(parse-line "$image")
  echo "${image_dir}/gather-static.sh"
}

setup(){
  for image in "${IMAGES[@]}"; do
    gather_static_file_script="$(gather-static-script $image)"
    if [[ -f $gather_static_file_script ]]; then
      $gather_static_file_script
    fi
  done
}

teardown(){
  for image in "${IMAGES[@]}"; do
    gather_static_file_script="$(gather-static-script $image)"
    if [[ -f $gather_static_file_script ]]; then
      CLEAN=true $gather_static_file_script
    fi
  done
}

parse-line() {
  parts=(${1//;/ })
  image_dir="${parts[0]}"
  arch="${DEFAULT_ARCH}"
  if [[ "${#parts[@]}" -gt 1 ]]; then
    arch="${parts[1]}"
  fi
  echo "$image_dir $arch"
}

push-images(){
  common_publish_args=()
  common_imgs=()
  for image in "${IMAGES[@]}"; do
    if [[ "$image" == "" || "$image" =~ \#.* ]]; then
      continue
    fi
    read image_dir arch < <(parse-line "$image")
    name="$(basename "${image_dir}")"

    publish_args=(--tarball=_bin/"${name}".tar --push=false)
    if [[ "${PUSH}" != 'false' ]]; then
        publish_args=(--push=true)
    fi
    # specify tag
    push_tags="$(tags-arg $arch)"
    publish_args+=(--base-import-paths ${push_tags} --platform="${arch}")

    # grouping images for better performance. To avoid introducing
    # map[arch]{imgs...} in bash, only grouping images aginst linux/amd64 for
    # now, which represent > 90% of the images so the performance gain should
    # be good for now.
    # No grouping if images are built locally
    if [[ "${arch}" != "${DEFAULT_ARCH}" || "${PUSH}" == 'false' ]]; then
      (set -x; _bin/ko publish "${publish_args[@]}" ./"${image_dir}")
    else
      # Group images with the same tag for ko, so that ko can process them in
      # parallel
      common_publish_args=(${publish_args[@]})
      common_imgs+=(./"${image_dir}")
    fi
  done

  # Now build common images
  if [[ "${#common_imgs[@]}" -gt 0 ]]; then
    (set -x; _bin/ko publish --jobs=10 "${common_publish_args[@]}" "${common_imgs[@]}")
  fi
}

main() {
  trap "teardown" exit
  setup
  push-images
}

main
