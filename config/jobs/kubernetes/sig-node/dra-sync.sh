#!/usr/bin/env bash
# Copyright 2024 The Kubernetes Authors.
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

# Running this script will automatically take changes made to
# dynamic-resource-allocation-canary.yaml since the last sync
# (tracked in .dra-sync-settings) and create a commit which
# applies those changes to dynamic-resource-allocation.yaml.

set -o errexit
set -o nounset
set -o pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd -P)"
cd "${REPO_ROOT}"

# get "last_sync"
source "config/jobs/kubernetes/sig-node/.dra-sync-settings"

if [ -n "$(git diff --cached 2>&1)" ]; then
    echo >&2 "ERROR: The git staging area must be clean."
    exit 1
fi

new_sync=$(git rev-parse HEAD)

diff=$(git diff ${last_sync}..${new_sync} config/jobs/kubernetes/sig-node/dynamic-resource-allocation-canary.yaml | sed -e 's/-canary//g')

if [ -z "${diff}" ]; then
    echo "No changes since last sync, nothing to do."
    exit 0
fi

# Generate a "git format-patch" alike patch and apply it.
git am <<EOF
From ${new_sync}
From: dra-sync.sh helper script <k8s-ci-robot@users.noreply.github.com>
Date: $(date --rfc-email)
Subject: [PATCH 1/1] dra: apply changes from canary jobs

---
${diff}
$(diff -u config/jobs/kubernetes/sig-node/.dra-sync-settings <(sed -e "s/last_sync=.*/last_sync=${new_sync}/" config/jobs/kubernetes/sig-node/.dra-sync-settings) | sed -e 's;^--- .*;--- a/config/jobs/kubernetes/sig-node/.dra-sync-settings;' -e 's;+++ .*;+++ b/config/jobs/kubernetes/sig-node/.dra-sync-settings;')
EOF

git log -p -n1

cat <<EOF

Commit created successfully (see above). Review and submit as a pull request.
EOF
