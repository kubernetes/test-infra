# experiment/gcp-nvkind

Shell helpers for Prow jobs that need:

1. A GCP project leased from a Boskos pool (e.g. `gpu-project`).
2. A GCE VM with an attached NVIDIA GPU.
3. A `kind` cluster managed by [nvkind](https://github.com/NVIDIA/nvkind)
   so containers get GPU access via the nvidia-container-runtime.

Typical callers install their own workload on top (for example a DRA driver
plus GPU Operator, a device-plugin variant, or CDI validation tooling).

## Layout

```
experiment/gcp-nvkind/
├── README.md
└── lib/
    ├── boskos.sh               # boskos::acquire / heartbeat_start / release
    ├── gce.sh                  # gce::create / ssh / scp_to / scp_from / wait_for_driver / delete
    ├── setup-nvkind-node.sh    # on-VM: install docker+toolkit+go+kind+nvkind+helm+kubectl; create cluster
    └── nvkind-config.yaml.tmpl # kind config: DRA feature gate + CDI containerd patch
```

## Contract

- Caller sets `BOSKOS_HOST`, `BOSKOS_RESOURCE_TYPE`, `JOB_NAME`.
- Caller's service account must have Compute IAM on the leased project.
- Caller runs `go install sigs.k8s.io/boskos/cmd/boskosctl@latest` at job
  start (not pre-installed in `kubekins-e2e`).
- After `setup-nvkind-node.sh` exits, the cluster is Ready at
  `$NVKIND_K8S_VERSION` with `KUBECONFIG=$HOME/.kube/config` on the VM.
- These helpers install no project-specific workload — that is the caller's
  job.

## Example

```bash
#!/usr/bin/env bash
set -euo pipefail
source "${TESTINFRA_DIR}/experiment/gcp-nvkind/lib/boskos.sh"
source "${TESTINFRA_DIR}/experiment/gcp-nvkind/lib/gce.sh"

boskos::acquire
export GCP_PROJECT="${BOSKOS_PROJECT}"
boskos::heartbeat_start

export VM_NAME="myjob-${BUILD_ID}"
export GCE_ZONE=us-central1-b
trap 'gce::delete; boskos::release' EXIT

gce::create
gce::wait_for_driver
gce::scp_to "${MY_TARBALL}" /tmp/
gce::ssh "tar -xzf /tmp/*.tgz -C /tmp/src"
gce::ssh "NVKIND_CLUSTER_NAME=test \
          NVKIND_K8S_VERSION=v1.34.3 \
          NVKIND_CONFIG_PATH=/tmp/src/.../nvkind-config.yaml.tmpl \
          bash /tmp/src/.../setup-nvkind-node.sh"
gce::ssh "bash /tmp/install-workload.sh && bash /tmp/run-tests.sh"
```
