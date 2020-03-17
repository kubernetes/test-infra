#!/bin/bash
#
# Copyright 2020 IBM Corporation
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

# This script build and push multiarch(amd64, ppc64le and s390x) image for the one specified by
# IMAGE_REPO, IMAGE_NAME and VERSION.
# It assumes the specified image for each platform is already pushed into corresponding docker registry.

ALL_PLATFORMS="amd64 ppc64le s390x"

IMAGE_REPO=${1}
IMAGE_NAME=${2}
VERSION=${3-"$(date +v%Y%m%d)-$(git describe --tags --always --dirty)"}

MAX_PULLING_RETRY=${MAX_PULLING_RETRY-10}
RETRY_INTERVAL=${RETRY_INTERVAL-10}

for arch in ${ALL_PLATFORMS}
do
    for i in $(seq 1 "${MAX_PULLING_RETRY}")
    do
        echo "Trying to pull image '${IMAGE_REPO}'/'${IMAGE_NAME}'-'${arch}':'${VERSION}'..."
        docker pull "${IMAGE_REPO}"/"${IMAGE_NAME}"-"${arch}":"${VERSION}" && break
        sleep "${RETRY_INTERVAL}"
        if [ "${i}" -eq "${MAX_PULLING_RETRY}" ]; then
            echo "Failed to pull image '${IMAGE_REPO}'/'${IMAGE_NAME}'-'${arch}':'${VERSION}'!!!"
            exit 1
        fi
    done
done

echo "Trying to download and install manifest-tool..."
curl -L -o /tmp/manifest-tool https://github.com/estesp/manifest-tool/releases/download/v1.0.0/manifest-tool-linux-amd64
chmod +x /tmp/manifest-tool

echo "Trying to build multiarch image for '${IMAGE_REPO}'/'${IMAGE_NAME}':'${VERSION}'..."
/tmp/manifest-tool push from-args --platforms linux/amd64,linux/ppc64le,linux/s390x --template "${IMAGE_REPO}"/"${IMAGE_NAME}"-ARCH:"${VERSION}" --target "${IMAGE_REPO}"/"${IMAGE_NAME}"
/tmp/manifest-tool push from-args --platforms linux/amd64,linux/ppc64le,linux/s390x --template "${IMAGE_REPO}"/"${IMAGE_NAME}"-ARCH:"${VERSION}" --target "${IMAGE_REPO}"/"${IMAGE_NAME}":"${VERSION}"
