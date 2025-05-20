#!/usr/bin/env bash

# Copyright 2025 The Kubernetes Authors.
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

set -o errexit
set -o nounset
set -o pipefail
set -o xtrace

# install kind
curl -sSL https://kind.sigs.k8s.io/dl/latest/linux-amd64.tgz | tar xvfz - -C "${PATH%%:*}/"

# install depstat
export WORKDIR=${ARTIFACTS:-$TMPDIR}
export PATH=$PATH:$GOPATH/bin
mkdir -p "${WORKDIR}"
pushd "$WORKDIR"
go install github.com/kubernetes-sigs/depstat@latest
go install github.com/sgaunet/mdtohtml@latest
popd

# needed by gomod_staleness.py
apt update && apt -y install jq

# grab the stats before we start
depstat stats -m "k8s.io/kubernetes$(ls staging/src/k8s.io | awk '{printf ",k8s.io/" $0}')" -v > "${WORKDIR}/stats-before.txt"

# Update each dependency to the latest version
./../test-infra/experiment/dependencies/gomod_staleness.py --patch-output "${WORKDIR}"/latest-go-mod-sum.patch --markdown-output "${WORKDIR}"/differences.md
mdtohtml "${WORKDIR}"/differences.md "${WORKDIR}"/differences.html

# See if update-vendor still works
hack/update-vendor.sh

# gather stats for comparison after running update-vendor
depstat stats -m "k8s.io/kubernetes$(ls staging/src/k8s.io | awk '{printf ",k8s.io/" $0}')" -v > "${WORKDIR}/stats-after.txt"
diff -s -u --ignore-all-space "${WORKDIR}"/stats-before.txt "${WORKDIR}"/stats-after.txt || true

# Do not worry if this fails, it is bound to fail
hack/lint-dependencies.sh || true

# Do not worry if this fails, it is bound to fail
hack/verify-typecheck.sh || true

# run kind based tests
e2e-k8s.sh || true
