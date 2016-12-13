#!/bin/bash

# Copyright 2015 The Kubernetes Authors.
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

# This script downloads, installs cloud sdk and activates the service account.
# USAGE:
#   gcloud-common.sh [--download] -t|--tar [gcloud-tarball-dir] -d|--dir [install-dir]
#       --download - if need download gcloud tarball from web
#       -t|--tar - where to find/fetch gcloud tarball
#       -d|--dir - where to install gcloud to
#


# default
DOWNLOAD=false
TARBALL="${WORKSPACE}/google-cloud-sdk.tar.gz"
INSTALL_DIR="/"

while [[ $# -gt 1 ]]
do
key="$1"

case $key in
    --download)
    DOWNLOAD=true # past argument
    ;;
    -t|--tar)
    TARBALL="$2"
    shift # past argument
    ;;
    -i|--install)
    INSTALL_DIR="$2"
    shift # past argument
    ;;
    *)
    # unknown option
    ;;
esac
shift # past argument or value
done

if [[ "$DOWNLOAD"==true ]]; then
  curl -fsSL --retry 3 --keepalive-time 2 -o "${TARBALL}" 'https://dl.google.com/dl/cloudsdk/channels/rapid/google-cloud-sdk.tar.gz'
fi

mkdir -p "${INSTALL_DIR}"
tar xzf "${TARBALL}" -C "${INSTALL_DIR}"

export CLOUDSDK_CORE_DISABLE_PROMPTS=1
"${INSTALL_DIR}/google-cloud-sdk/install.sh" --disable-installation-options --bash-completion=false --path-update=false --usage-reporting=false
export PATH=${INSTALL_DIR}/google-cloud-sdk/bin:${PATH}
gcloud components install alpha
gcloud components install beta
gcloud info

# activate gcloud
if [[ -n "${GOOGLE_APPLICATION_CREDENTIALS:-}" ]]; then
  gcloud auth activate-service-account --key-file="${GOOGLE_APPLICATION_CREDENTIALS}"
fi

