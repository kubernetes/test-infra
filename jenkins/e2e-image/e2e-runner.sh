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

# Run e2e tests using environment variables exported in e2e.sh.

set -o errexit
set -o nounset
set -o pipefail
set -o xtrace

export PS4='+(${BASH_SOURCE}:${LINENO}): ${FUNCNAME[0]:+${FUNCNAME[0]}(): }'

# Have cmd/e2e run by goe2e.sh generate JUnit report in ${WORKSPACE}/junit*.xml
ARTIFACTS=${WORKSPACE}/_artifacts
mkdir -p ${ARTIFACTS}

: ${KUBE_GCS_RELEASE_BUCKET:="kubernetes-release"}
: ${KUBE_GCS_DEV_RELEASE_BUCKET:="kubernetes-release-dev"}
: ${JENKINS_SOAK_PREFIX:="gs://kubernetes-jenkins/soak/${JOB_NAME}"}
: ${JENKINS_FEDERATION_PREFIX:="gs://kubernetes-jenkins/federation/${JOB_NAME}"}

# Explicitly set config path so staging gcloud (if installed) uses same path
export CLOUDSDK_CONFIG="${WORKSPACE}/.config/gcloud"

echo "--------------------------------------------------------------------------------"
echo "Test Environment:"
printenv | sort
echo "--------------------------------------------------------------------------------"

# When run inside Docker, we need to make sure all files are world-readable
# (since they will be owned by root on the host).
trap "chmod -R o+r '${ARTIFACTS}'" EXIT SIGINT SIGTERM
export E2E_REPORT_DIR=${ARTIFACTS}

if [[ "${FEDERATION:-}" == "true" ]]; then
  FEDERATION_UP="${FEDERATION_UP:-true}"
  FEDERATION_DOWN="${FEDERATION_DOWN:-true}"
fi

e2e_go_args=( \
  -v \
  --dump="${ARTIFACTS}" \
)

# Allow download & unpack of alternate version of tests, for cross-version & upgrade testing.
#
# JENKINS_PUBLISHED_SKEW_VERSION adds a second --extract before the other one.
# The JENKINS_PUBLISHED_SKEW_VERSION extracts to kubernetes_skew.
# The JENKINS_PUBLISHED_VERSION extracts to kubernetes.
#
# For upgrades, PUBLISHED_SKEW should be a new release than PUBLISHED.
if [[ -n "${JENKINS_PUBLISHED_SKEW_VERSION:-}" ]]; then
  e2e_go_args+=(--extract="${JENKINS_PUBLISHED_SKEW_VERSION}")
  # Assume JENKINS_USE_SKEW_KUBECTL == true for backward compatibility
  : ${JENKINS_USE_SKEW_KUBECTL:=true}
  if [[ "${JENKINS_USE_SKEW_TESTS:-}" == "true" ]]; then
    e2e_go_args+=(--skew)  # Get kubectl as well as test code from kubernetes_skew
  elif [[ "${JENKINS_USE_SKEW_KUBECTL}" == "true" ]]; then
    # Append kubectl-path of skewed kubectl to test args, since we always
    # want that to use the skewed kubectl version:
    #  - for upgrade jobs, we want kubectl to be at the same version as
    #    master.
    #  - for client skew tests, we want to use the skewed kubectl
    #    (that's what we're testing).
    GINKGO_TEST_ARGS="${GINKGO_TEST_ARGS:-} --kubectl-path=$(pwd)/kubernetes_skew/cluster/kubectl.sh"
  fi
fi


# We get the Kubernetes tarballs unless we are going to use old ones
if [[ "${JENKINS_USE_EXISTING_BINARIES:-}" =~ ^[yY]$ ]]; then
  echo "Using existing binaries; not cleaning, fetching, or unpacking new ones."
elif [[ "${JENKINS_USE_LOCAL_BINARIES:-}" =~ ^[yY]$ ]]; then
  e2e_go_args+=(--extract="local")
elif [[ "${JENKINS_USE_SERVER_VERSION:-}" =~ ^[yY]$ ]]; then
  # This is for test, staging, and prod jobs on GKE, where we want to
  # test what's running in GKE by default rather than some CI build.
  e2e_go_args+=(--extract="gke")
elif [[ "${JENKINS_USE_GCI_VERSION:-}" =~ ^[yY]$ ]]; then
  # Use GCI image builtin version. Needed for GCI release qual tests.
  e2e_go_args+=(--extract="gci/${JENKINS_GCI_HEAD_IMAGE_FAMILY}")
else
  # use JENKINS_PUBLISHED_VERSION, default to 'ci/latest', since that's
  # usually what we're testing.
  e2e_go_args+=(--extract="${JENKINS_PUBLISHED_VERSION:-ci/latest}")
fi

if [[ "${JENKINS_SOAK_MODE:-}" == "y" ]]; then
  # In soak mode we sync cluster state to gcs.
  # If we --up a cluster, we save the kubecfg and version info to gcs.
  # Otherwise we load kubecfg and version info from gcs.
  e2e_go_args+=(--save="${JENKINS_SOAK_PREFIX}")
elif [[ "${FEDERATION_UP:-}" != "true" && -n "${FEDERATION_CLUSTERS:-}" ]]; then
  # If we are only deploying federated clusters without the federation control plane,
  # then the kubeconfig for these clusters will be required while deploying the federation
  # control plane. So we persist the kubeconfig in GCS for later use.
  e2e_go_args+=(--save="${JENKINS_FEDERATION_PREFIX}")
elif [[ "${FEDERATION_UP:-}" == "true" && -z "${FEDERATION_CLUSTERS:-}" ]]; then
  # If we are only deploying a federation control plane without the federated
  # clusters, then we assume that the federated clusters are already deployed
  # and their kubeconfig is stored in GCS. We copy that kubeconfig from GCS
  # to the local machine to operate on those clusters.
  #
  # Note: This elif block and the previous one are essentially the same. The
  # real logic that decides whether this is a store or a load is in kubetest.
  # We could have merged the two elif blocks. However, we are keeping them
  # separate for clarity.
  e2e_go_args+=(--save="${JENKINS_FEDERATION_PREFIX}")
fi

if [[ "${FAIL_ON_GCP_RESOURCE_LEAK:-true}" == "true" ]]; then
  case "${KUBERNETES_PROVIDER}" in
    gce|gke)
      e2e_go_args+=(--check-leaked-resources)
      ;;
  esac
fi

if [[ "${E2E_UP:-}" == "true" ]] || [[ "${FEDERATION_UP:-}" == "true" ]]; then
  e2e_go_args+=(--up)
fi

if [[ "${E2E_DOWN:-}" == "true" ]] || [[ "${FEDERATION_DOWN:-}" == "true" ]]; then
  e2e_go_args+=(--down)
fi

if [[ "${FEDERATION_UP:-}" == "true" ]] || [[ "${FEDERATION_DOWN:-}" == "true" ]] || [[ "${FEDERATION:-}" == "true" ]]; then
  e2e_go_args+=(--federation)
  if [[ -z "${FEDERATION_CLUSTERS:-}" ]]; then
    e2e_go_args+=("--deployment=none")
  fi
fi

if [[ "${E2E_TEST:-}" == "true" ]]; then
  e2e_go_args+=(--test)
  if [[ -n "${GINKGO_TEST_ARGS:-}" ]]; then
    e2e_go_args+=(--test_args="${GINKGO_TEST_ARGS}")
  fi
fi

# Optionally run upgrade tests before other tests.
if [[ "${E2E_UPGRADE_TEST:-}" == "true" ]]; then
  e2e_go_args+=(--upgrade_args="${GINKGO_UPGRADE_TEST_ARGS}")
fi

if [[ -n "${KUBEKINS_TIMEOUT:-}" ]]; then
  e2e_go_args+=(--timeout="${KUBEKINS_TIMEOUT}")
fi

if [[ -n "${E2E_PUBLISH_PATH:-}" ]]; then
  e2e_go_args+=(--publish="${E2E_PUBLISH_PATH}")
fi

if [[ "${E2E_PUBLISH_GREEN_VERSION:-}" == "true" ]]; then
  # Use plaintext version file packaged with kubernetes.tar.gz
  e2e_go_args+=(--publish="gs://${KUBE_GCS_DEV_RELEASE_BUCKET}/ci/latest-green.txt")
fi

kubetest ${E2E_OPT:-} "${e2e_go_args[@]}" "${@}"
