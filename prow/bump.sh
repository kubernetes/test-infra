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

color-error() { # Light red
  echo -e "\x1B[91m${@}\x1B[0m"
}

color-target() { # Bold cyan
  echo -e "\x1B[1;33m${@}\x1B[0m"
}

# darwin is great
SED=sed
if which gsed &>/dev/null; then
  SED=gsed
fi
if ! ($SED --version 2>&1 | grep -q GNU); then
  echo "!!! GNU sed is required.  If on OS X, use 'brew install gnu-sed'." >&2
  exit 1
fi

cd "$(dirname "${BASH_SOURCE}")"

usage() {
  echo "Usage: "$(basename "$0")" [--push || --list || vYYYYMMDD-deadbeef] [image subset...]" >&2
  exit 1
}

if [[ -n "${GOOGLE_APPLICATION_CREDENTIALS:-}" ]]; then
  echo "Detected GOOGLE_APPLICATION_CREDENTIALS, activating..." >&2
  gcloud auth activate-service-account --key-file="$GOOGLE_APPLICATION_CREDENTIALS"
  gcloud auth configure-docker
  cmd="--push"
elif [[ $# == 0 ]]; then
  usage
else
  cmd="$1"
  shift
fi

if [[ "$cmd" == "--push" ]]; then
  new_version="v$(date -u '+%Y%m%d')-$(git describe --tags --always --dirty)"
  echo -e "version: $(color-version ${new_version})" >&2
  if [[ "${new_version}" == *-dirty ]]; then
    echo -e "$(color-error ERROR): uncommitted changes to repo" >&2
    echo "  Fix with git commit" >&2
    exit 1
  fi
  echo -e "Pushing $(color-version ${new_version}) via $(color-target //prow:release-push --platforms=@io_bazel_rules_go//go/toolchain:linux_amd64) ..." >&2
  bazel run //prow:release-push --platforms=@io_bazel_rules_go//go/toolchain:linux_amd64
elif [[ "$cmd" == "--list" ]]; then
  # TODO(fejta): figure out why the timestamp on all these image is 1969...
  # Then we'll be able to just sort them.
  options=(
    $(gcloud container images list-tags gcr.io/k8s-prow/plank --filter="tags ~ ^v|,v" --format='value(tags)' \
      | grep -o -E 'v\d{8}-(\d|[da-f]){6,9}' | sort -u | tail -n 10)
  )
  echo "Recent versions of prow:" >&2
  new_version=
  for o in "${options[@]}"; do
    def_opt="$o"
    echo -e "  $(color-version $o)"
  done
  read -p "select version [$def_opt]: " new_version
  if [[ -z "${new_version:-}" ]]; then
    new_version="$def_opt"
  else
    found=
    for o in "${options[@]}"; do
      if [[ "$o" == "$new_version" ]]; then
        found=yes
        break
      fi
    done
    if [[ -z "$found" ]]; then
      echo "Invalid version: $new_version" >&2
      exit 1
    fi
  fi
elif [[ "$cmd" =~ v[0-9]{8}-[a-f0-9]{6,9} ]]; then
  new_version="$cmd"
else
  usage
fi

if [[ -n "${GOOGLE_APPLICATION_CREDENTIALS:-}" ]]; then
  # TODO(fejta): consider making this publish to a recent-releases.json or something
  exit 0
fi


# Determine what deployment images we need to update
echo -n "images: " >&2
images=("$@")
if [[ "${#images[@]}" == 0 ]]; then
  echo "querying bazel for $(color-target :image) targets under $(color-target //prow/...) ..." >&2
  images=($(bazel query 'filter(".*:image", //prow/...)' | cut -d : -f 1 | xargs -n 1 basename))
  echo -n "images: " >&2
fi
echo -e "$(color-image ${images[@]})" >&2

echo -e "Bumping: $(color-image ${images[@]}) to $(color-version ${new_version}) ..." >&2

for i in "${images[@]}"; do
  echo -e "  $(color-image $i): $(color-version $new_version)" >&2
  $SED -i "s/\(${i}:\)v[a-f0-9-]\+/\1${new_version}/I" cluster/*.yaml
done

echo "Deploy with:" >&2
echo -e "  $(color-target bazel run //prow/cluster:production.apply --platforms=@io_bazel_rules_go//go/toolchain:linux_amd64)"
