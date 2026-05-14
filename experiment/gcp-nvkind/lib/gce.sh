#!/usr/bin/env bash
# Copyright The Kubernetes Authors.
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

# gce.sh -- GCE VM lifecycle: create, wait for NVIDIA driver, SSH, SCP, delete.
#
# Required env: GCP_PROJECT, GCE_ZONE, VM_NAME.
# Optional env (with defaults):
#   GCE_MACHINE_TYPE    n1-standard-4
#   GCE_ACCELERATOR     type=nvidia-tesla-t4,count=1
#   GCE_IMAGE_FAMILY    common-cu129-ubuntu-2204-nvidia-580
#   GCE_IMAGE_PROJECT   deeplearning-platform-release
#   GCE_BOOT_DISK_SIZE  100GB

set -o errexit
set -o nounset
set -o pipefail

: "${GCE_MACHINE_TYPE:=n1-standard-4}"
: "${GCE_ACCELERATOR:=type=nvidia-tesla-t4,count=1}"
: "${GCE_IMAGE_FAMILY:=common-cu129-ubuntu-2204-nvidia-580}"
: "${GCE_IMAGE_PROJECT:=deeplearning-platform-release}"
: "${GCE_BOOT_DISK_SIZE:=100GB}"

gce::create() {
  : "${GCP_PROJECT:?GCP_PROJECT must be set}"
  : "${GCE_ZONE:?GCE_ZONE must be set}"
  : "${VM_NAME:?VM_NAME must be set}"

  echo "Creating VM ${VM_NAME} in ${GCP_PROJECT}/${GCE_ZONE}..."
  gcloud compute instances create "${VM_NAME}" \
    --project="${GCP_PROJECT}" \
    --zone="${GCE_ZONE}" \
    --machine-type="${GCE_MACHINE_TYPE}" \
    --accelerator="${GCE_ACCELERATOR}" \
    --maintenance-policy=TERMINATE \
    --image-family="${GCE_IMAGE_FAMILY}" \
    --image-project="${GCE_IMAGE_PROJECT}" \
    --metadata=install-nvidia-driver=True \
    --boot-disk-size="${GCE_BOOT_DISK_SIZE}" \
    --boot-disk-type=pd-balanced
}

gce::wait_for_driver() {
  local deadline
  deadline=$(( $(date +%s) + 900 ))
  echo "Waiting for nvidia-smi to succeed on ${VM_NAME} (up to 15 min)..."
  until gce::ssh 'nvidia-smi -L' 2>&1 | grep -q 'GPU 0'; do
    local now
    now=$(date +%s)
    if [ "${now}" -gt "${deadline}" ]; then
      echo "TIMEOUT waiting for driver on ${VM_NAME}"
      return 1
    fi
    echo "  driver not ready yet ($(( (deadline - now) / 60 )) min left)"
    sleep 15
  done
  echo "Driver ready: $(gce::ssh 'nvidia-smi -L' | head -1)"
}

gce::ssh() {
  gcloud compute ssh "${VM_NAME}" \
    --zone="${GCE_ZONE}" \
    --project="${GCP_PROJECT}" \
    --ssh-flag='-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR -o ServerAliveInterval=30' \
    --command="$*"
}

gce::scp_to() {
  # gce::scp_to LOCAL... REMOTE_PATH
  local n=$#
  local remote="${!n}"
  local locals=()
  local i
  for ((i=1; i<n; i++)); do locals+=("${!i}"); done
  gcloud compute scp --recurse \
    --zone="${GCE_ZONE}" \
    --project="${GCP_PROJECT}" \
    "${locals[@]}" "${VM_NAME}:${remote}"
}

gce::scp_from() {
  # gce::scp_from REMOTE_PATH LOCAL_PATH
  gcloud compute scp --recurse \
    --zone="${GCE_ZONE}" \
    --project="${GCP_PROJECT}" \
    "${VM_NAME}:$1" "$2"
}

gce::delete() {
  if [ -n "${VM_NAME:-}" ] && [ -n "${GCE_ZONE:-}" ] && [ -n "${GCP_PROJECT:-}" ]; then
    echo "Deleting VM ${VM_NAME}..."
    gcloud compute instances delete "${VM_NAME}" \
      --project="${GCP_PROJECT}" --zone="${GCE_ZONE}" \
      --quiet 2>/dev/null || true
  fi
}
