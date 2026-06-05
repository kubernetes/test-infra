#!/bin/bash

# Copyright 2026 The Kubernetes Authors.
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

# GCE startup-script that side-loads a container image archive onto a node e2e
# VM, so a test can exercise a locally built image instead of pulling one from a
# registry.
#
# Wire it in with the image-config metadata, e.g.
#   --instance-metadata=startup-script<.../sideload-image.sh,sideload-image-url=<url>
# The archive (an OCI or docker-archive tarball) is imported into the CRI
# (k8s.io) containerd namespace. Combine with --prepull-images=false and the
# test pod's default IfNotPresent policy so the imported image is used with no
# registry involved. No-op when sideload-image-url is unset.

set -o errexit
set -o nounset
set -o pipefail

readonly META="http://metadata.google.internal/computeMetadata/v1/instance/attributes"

url="$(curl -sf -H 'Metadata-Flavor: Google' "${META}/sideload-image-url" || true)"
if [[ -z "${url}" ]]; then
  echo "sideload-image-url instance metadata not set; nothing to side-load"
  exit 0
fi

echo "Downloading image archive from ${url}"
curl -fsSL --retry 5 --retry-delay 3 -o /tmp/sideload-image.tar "${url}"

# init.yaml restarts containerd near the end of node setup; wait for it before
# importing into the CRI (k8s.io) namespace that the kubelet uses.
for _ in $(seq 1 90); do
  if ctr -n k8s.io version >/dev/null 2>&1; then
    break
  fi
  sleep 2
done

echo "Importing side-loaded image into containerd"
ctr -n k8s.io images import /tmp/sideload-image.tar
