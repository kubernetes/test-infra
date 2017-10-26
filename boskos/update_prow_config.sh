#!/usr/bin/env bash
# Copyright 2017 The Kubernetes Authors.
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

# Usage: updade_prow_config.sh

set -o errexit
set -o nounset
set -o pipefail

# Script triggered by prow postsubmit job
# Update boskos configmap in prow

TREE="$(dirname ${BASH_SOURCE[0]})/.."

if [[ -a "${PROW_SERVICE_ACCOUNT:-}" ]] ; then
	gcloud auth activate-service-account --key-file="${PROW_SERVICE_ACCOUNT}"
fi

if ! [ -x "$(command -v kubectl)" ]; then
	gcloud components install kubectl 
fi

pushd "${TREE}/boskos"
make update-config
popd

# switch back to default service account for uploading logs
if [[ -a "${GOOGLE_APPLICATION_CREDENTIALS:-}" ]] ; then
	gcloud auth activate-service-account --key-file="${GOOGLE_APPLICATION_CREDENTIALS}"
fi
