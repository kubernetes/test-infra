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

# One-shot KRTE cgroup topology snapshot. Records four layers (outer wrapper,
# dockerd, an ordinary Docker child, and a kind node with a control-plane
# process) so #37775 can decide the fix. It is a collector, not a contract:
# it fails closed if a promised layer cannot run, but it does not assert a
# topology, because the expected topology is exactly what this run establishes.

set -o errexit
set -o nounset
set -o pipefail

artifacts="${ARTIFACTS:-/tmp/krte-cgroup-contract}"
mkdir -p "${artifacts}"
tmp="$(mktemp -d)"
probe="${tmp}/probe"
container="krte-cgroup-contract-${RANDOM}-$$"
kind_name="krte-contract"
forced="GODEBUG=containermaxprocs=1,updatemaxprocs=1"

cleanup() {
  kind delete cluster --name "${kind_name}" >/dev/null 2>&1 || true
  docker rm -f "${container}" >/dev/null 2>&1 || true
  rm -rf "${tmp}"
}
trap cleanup EXIT

log() { echo "[krte-cgroup-contract] $*"; }

# --- provenance: what image and toolchain actually ran -----------------------
{
  printf 'krte_image=%s\n' "${KRTE_IMAGE:-unknown}"
  printf 'kind_version=%s\n' "$(kind version 2>/dev/null || echo unavailable)"
  printf 'go_version=%s\n' "$(go version 2>/dev/null || echo unavailable)"
} >"${artifacts}/provenance.txt"

# --- build the probe ---------------------------------------------------------
CGO_ENABLED=0 go build -trimpath -o "${probe}" .

# --- layer 1: outer wrapper command, default and forced ----------------------
"${probe}" -label=outer-default -output="${artifacts}/outer-default.json"
env "${forced}" "${probe}" -label=outer -output="${artifacts}/outer.json"
cat /proc/self/cgroup >"${artifacts}/outer.cgroup"
cat /proc/self/mountinfo >"${artifacts}/outer.mountinfo"

# --- layer 2: dockerd's own cgroup (why children land where they do) ---------
if dockerd_pid="$(pidof dockerd | awk '{print $1}')" && [[ -n "${dockerd_pid}" ]]; then
  cat "/proc/${dockerd_pid}/cgroup" >"${artifacts}/dockerd-from-outer.cgroup" 2>/dev/null || true
  for ns in cgroup mnt pid; do
    printf '%s=%s\n' "${ns}" "$(readlink "/proc/${dockerd_pid}/ns/${ns}" 2>/dev/null || echo unknown)"
  done >"${artifacts}/dockerd.ns"
else
  log "dockerd pid not found; skipping dockerd cgroup capture"
fi

# --- layer 3: an ordinary nested Docker child, fail closed -------------------
# The probe was built to ${tmp}/probe, which is the build context, so the
# scratch image can COPY it directly.
cat >"${tmp}/Dockerfile" <<'DOCKERFILE'
FROM scratch
COPY probe /probe
ENTRYPOINT ["/probe"]
DOCKERFILE
timeout 3m docker build --pull=false -t krte-cgroup-contract:local "${tmp}"

# Detached, no --rm: the cleanup trap removes it, and keeping it lets us read
# the exit code and logs when the probe fails.
cid="$(docker run --detach \
  --name "${container}" \
  --volume "${artifacts}:/artifacts" \
  --env "${forced}" \
  krte-cgroup-contract:local \
  -label=docker-child -output=/artifacts/docker-child.json -hold=30s)"

for _ in $(seq 1 100); do
  [[ -s "${artifacts}/docker-child.json" ]] && break
  if [[ "$(docker inspect --format '{{.State.Running}}' "${cid}")" != "true" ]]; then
    break
  fi
  sleep 0.1
done

docker logs "${cid}" >"${artifacts}/docker-child.log" 2>&1 || true
if [[ ! -s "${artifacts}/docker-child.json" ]]; then
  child_rc="$(docker inspect --format '{{.State.ExitCode}}' "${cid}" 2>/dev/null || echo unknown)"
  log "docker child produced no snapshot (exit ${child_rc}); see docker-child.log"
  exit 1
fi

# A second child with no forced GODEBUG shows the default runtime behavior.
docker run --rm --volume "${artifacts}:/artifacts" \
  krte-cgroup-contract:local \
  -label=docker-child-default -output=/artifacts/docker-child-default.json >/dev/null

# host-side view of the child cgroup, and only the Docker fields we compare
pid="$(docker inspect --format '{{.State.Pid}}' "${cid}")"
cat "/proc/${pid}/cgroup" >"${artifacts}/docker-child-from-outer.cgroup"
{
  printf 'server_version=%s\n' "$(timeout 30s docker info --format '{{.ServerVersion}}')"
  printf 'cgroup_driver=%s\n' "$(timeout 30s docker info --format '{{.CgroupDriver}}')"
  printf 'cgroup_version=%s\n' "$(timeout 30s docker info --format '{{.CgroupVersion}}')"
} >"${artifacts}/docker-info.txt"
docker inspect --format '{{json .State}}' "${cid}" >"${artifacts}/docker-child-state.json"
docker inspect --format '{{.HostConfig.CgroupParent}}' "${cid}" >"${artifacts}/docker-child-cgroup-parent.txt"
docker inspect --format '{{.HostConfig.CgroupnsMode}}' "${cid}" >"${artifacts}/docker-child-cgroupns-mode.txt"

child_rc="$(docker wait "${cid}")"
if [[ "${child_rc}" != "0" ]]; then
  log "docker child probe exited ${child_rc}; see docker-child.log"
  exit 1
fi

# --- layer 4: a kind node and one Go control-plane process -------------------
# In a real KRTE presubmit dockerd is already up. A missing or unhealthy kind
# means this layer did not run, so capture the log and then fail rather than
# report a green run that skipped it.
command -v kind >/dev/null || { log "kind binary is required for the kind-node layer"; exit 1; }
if timeout 10m kind create cluster --name "${kind_name}" --wait 3m >"${artifacts}/kind-create.log" 2>&1; then
  node="$(docker ps --filter "name=${kind_name}-control-plane" --format '{{.ID}}' | head -1)"
  if [[ -n "${node}" ]]; then
    docker cp "${probe}" "${node}:/probe"
    # A Go process in the node: what GOMAXPROCS it is handed. This is the
    # environmental default the probe observes, not a running component's value.
    docker exec "${node}" /probe -label=kind-node-go >"${artifacts}/kind-node-go.json" 2>&1 || true
    if [[ ! -s "${artifacts}/kind-node-go.json" ]]; then
      log "kind node probe produced no snapshot; see kind-node-go.json"
      exit 1
    fi
    docker exec "${node}" sh -c 'c=$(awk -F: "\$1==0{print \$3}" /proc/1/cgroup); \
      printf "pid1_comm=%s\npid1_cgroup=%s\npid1_cpu_max=%s\n" \
      "$(cat /proc/1/comm)" "${c}" "$(cat /sys/fs/cgroup${c}/cpu.max 2>/dev/null || echo absent)"' \
      >"${artifacts}/kind-node-pid1.txt" 2>&1 || true
    api="$(docker exec "${node}" sh -c 'pidof kube-apiserver 2>/dev/null | tr " " "\n" | head -1' || true)"
    if [[ -n "${api}" ]]; then
      docker exec "${node}" sh -c "c=\$(awk -F: '\$1==0{print \$3}' /proc/${api}/cgroup); \
        printf 'apiserver_cgroup=%s\napiserver_leaf_cpu_max=%s\n' \
        \"\${c}\" \"\$(cat /sys/fs/cgroup\${c}/cpu.max 2>/dev/null || echo absent)\"" \
        >"${artifacts}/kind-apiserver.txt" 2>&1 || true
    else
      log "no kube-apiserver process found in the node"
    fi
  else
    log "kind reported success but no control-plane node was found"
    exit 1
  fi
else
  log "kind cluster did not come up; see kind-create.log"
  exit 1
fi

# --- summary of the two directly comparable snapshots ------------------------
"${probe}" -summarize "${artifacts}"
log "artifacts in ${artifacts}"
