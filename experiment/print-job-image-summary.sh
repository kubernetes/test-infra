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

# a silly script to audit how many jobs are using which images for prow.k8s.io

set -o errexit
set -o nounset
set -o pipefail

script_root=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)
repo_root=$(cd ${script_root}/.. && pwd)
jobs_dir="${1:-${repo_root}/config/jobs}"

# NOTE: assumes ripgrep
if ! command -v rg >/dev/null; then
  echo "please install ripgrep" >&2
  exit 1
fi

function images() {
  rg --no-filename --iglob "*.yaml" image: "${jobs_dir}" | sed -e 's|.*image: ||' | sort | cut -d: -f 1
}

function image_include_exclude() {
  local include="${1}"
  local exclude="${2}"
  local debug="${3:-false}"
  echo "$(images | grep -Ev "${exclude}" | grep -Ec "${include}") total, $(images | uniq | grep -Ev "${exclude}" | grep -Ec "${include}") unique"
  if ${debug}; then
    images | uniq | grep -Ev "${exclude}" | grep -E "${include}"
  fi
}

cat <<EOS
# images used by prowjobs on prow.k8s.io
- total /                      $(image_include_exclude "" "^$") 
  - not gcr.io /               $(image_include_exclude "" "gcr\.io")
    - dockerhub /              $(image_include_exclude "^[^\.]+$|^[^/]+$|docker\.io" "gcr\.io")
    - quay.io /                $(image_include_exclude "quay\.io" "gcr\.io")
  - gcr.io /                   $(image_include_exclude "gcr\.io" "^$")
    - kubernetes.io gcp org
      - k8s-staging            $(image_include_exclude "gcr\.io/k8s-staging" "^$")
      - k8s.gcr.io             $(image_include_exclude "k8s\.gcr\.io" "^$")
    - google.com gcp org
      - k8s-prow               $(image_include_exclude "gcr\.io/k8s-prow" "^$")
      - k8s-testimages         $(image_include_exclude "gcr\.io/k8s-testimages" "^$")
        - kubekins-e2e         $(image_include_exclude "gcr\.io/k8s-testimages/kubekins-e2e" "^$")
        - image-builder        $(image_include_exclude "gcr\.io/k8s-testimages/image-builder" "^$")
        - krte                 $(image_include_exclude "gcr\.io/k8s-testimages/krte" "^$")
        - other                $(image_include_exclude "gcr\.io/k8s-testimages" "kubekins-e2e|krte")
$(for i in $(image_include_exclude "gcr\.io/k8s-testimages" "kubekins-e2e" true | tail -n+2 | xargs -n1 basename); do printf \
"          - %-18s %s\n" "${i}" "$(image_include_exclude "gcr\.io/k8s-testimages/${i}" "^$")"; \
done)
    - other (unsure which org) $(image_include_exclude "gcr\.io" "^k8s.|k8s-(staging|prow|testimages)")
$(for i in $(image_include_exclude "gcr\.io" "^k8s.|k8s-(staging|prow|testimages)" true | tail -n+2 | xargs -n1 basename); do printf \
"      - %-22s %s\n" "${i}" "$(image_include_exclude "gcr\.io/.*/${i}$" "^$")"; \
done)
EOS
