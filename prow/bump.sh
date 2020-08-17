#!/bin/bash
# Copyright 2016 The Kubernetes Authors.
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

# bump.sh will
# * ensure there are no pending changes
# * optionally activate GOOGLE_APPLICATION_CREDENTIALS and configure-docker if set
# * run //prow:release-push to build and push prow images
# * update all the cluster/*.yaml files to use the new image tags

set -o errexit
set -o nounset
set -o pipefail

# TODO(fejta): rewrite this in a better language REAL SOON

# See https://misc.flogisoft.com/bash/tip_colors_and_formatting

color-image() { # Bold magenta
  echo -e "\x1B[1;35m${@}\x1B[0m"
}

color-version() { # Bold blue
  echo -e "\x1B[1;34m${@}\x1B[0m"
}

color-target() { # Bold cyan
  echo -e "\x1B[1;33m${@}\x1B[0m"
}

# darwin is great
SED=sed
if which gsed &>/dev/null; then
  SED=gsed
fi
if ! (${SED} --version 2>&1 | grep -q GNU); then
  echo "!!! GNU sed is required.  If on OS X, use 'brew install gnu-sed'." >&2
  exit 1
fi

TAC=tac
if which gtac &>/dev/null; then
  TAC=gtac
fi
if ! which "${TAC}" &>/dev/null; then
  echo "tac (reverse cat) required. If on OS X then 'brew install coreutils'." >&2
  exit 1
fi

cd "$(dirname "${BASH_SOURCE}")"

usage() {
  echo "Usage: "$(basename "$0")" [--list || --latest || vYYYYMMDD-deadbeef] [image subset...]" >&2
  exit 1
}

if [[ -n "${GOOGLE_APPLICATION_CREDENTIALS:-}" ]]; then
  echo "Detected GOOGLE_APPLICATION_CREDENTIALS, activating..." >&2
  gcloud auth activate-service-account --key-file="${GOOGLE_APPLICATION_CREDENTIALS}"
  gcloud auth configure-docker
fi

cmd=
if [[ $# != 0 ]]; then
  cmd="$1"
  shift
fi

# List the $1 most recently pushed prow versions
list-options() {
  count="$1"
  gcloud container images list-tags gcr.io/k8s-prow/plank --limit="${count}" --format='value(tags)' \
      | grep -o -E 'v[^,]+' | "${TAC}"
}

# Print 10 most recent prow versions, ask user to select one, which becomes new_version
list() {
  echo "Listing recent versions..." >&2
  echo "Recent versions of prow:" >&2
  options=(
    $(list-options 10)
    )
  if [[ -z "${options[@]}" ]]; then
    echo "No versions found" >&2
    exit 1
  fi
  new_version=
  for o in "${options[@]}"; do
    def_opt="${o}"
    echo -e "  $(color-version "${o}")"
  done
  read -p "Select version [$(color-version "${def_opt}")]: " new_version
  if [[ -z "${new_version:-}" ]]; then
    new_version="${def_opt}"
  else
    found=
    for o in "${options[@]}"; do
      if [[ "${o}" == "${new_version}" ]]; then
        found=yes
        break
      fi
    done
    if [[ -z "${found}" ]]; then
      echo "Invalid version: ${new_version}" >&2
      exit 1
    fi
  fi
}

if [[ "${cmd}" == "--push" ]]; then
  echo "WARNING: --push is deprecated please use push.sh instead"
  "$(dirname "$0")/push.sh"
  exit 0
fi

if [[ -z "${cmd}" || "${cmd}" == "--list" ]]; then
  list
elif [[ "${cmd}" =~ v[0-9]{8}-[a-f0-9]{6,9} ]]; then
  new_version="${cmd}"
elif [[ "${cmd}" == "--latest" ]]; then
  new_version="$(list-options 1)"
else
  usage
fi

# Determine what deployment images we need to update.
imagedirs="//prow/... + //label_sync/... + //ghproxy/... + //robots/commenter/... + //robots/issue-creator/..."
images=("$@")
if [[ "${#images[@]}" == 0 ]]; then
  echo -e "querying bazel for $(color-target :image) targets under $(color-target ${imagedirs}) ..." >&2
  images=($(bazel query "filter(\".*:image\", ${imagedirs})" | cut -d : -f 1 | xargs -n 1 basename))
  echo -n "images: " >&2
fi
echo -e "$(color-image ${images[@]})" >&2

echo -e "Bumping: $(color-image ${images[@]}) to $(color-version ${new_version}) ..." >&2

# Determine which files we need to update.
configfiles=($(grep -rl -e "gcr.io/k8s-prow/" ../config/jobs))
configfiles+=(cluster/*.yaml)
configfiles+=(../label_sync/cluster/*.yaml)
configfiles+=(cmd/branchprotector/*.yaml)
configfiles+=("config.yaml")

# Update image tags for the identified images in the identified files.
for i in "${images[@]}"; do
  echo -e "  $(color-image ${i}): $(color-version ${new_version})" >&2
  filter="s/gcr.io\/k8s-prow\/\(${i}:\)v[a-f0-9-]\+/gcr.io\/k8s-prow\/\1${new_version}/I"
  for cfg in "${configfiles[@]}"; do
    ${SED} -i "${filter}" ${cfg}
  done
done

echo "Deploy with:" >&2
echo -e "  $(color-target bazel run //config/prow/cluster:production.apply --platforms=@io_bazel_rules_go//go/toolchain:linux_amd64)"
