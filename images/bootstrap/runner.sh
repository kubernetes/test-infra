#!/usr/bin/env bash
# Copyright 2018 The Kubernetes Authors.
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

# generic runner script, handles DIND, bazelrc for caching, etc.

# Check if the job has opted-in to bazel remote caching and if so generate 
# .bazelrc entries pointing to the remote cache
export BAZEL_REMOTE_CACHE_ENABLED=${BAZEL_REMOTE_CACHE_ENABLED:-false}
if [[ "${BAZEL_REMOTE_CACHE_ENABLED}" == "true" ]]; then
    echo "Bazel remote cache is enabled, generating .bazelrcs ..."
    /usr/local/bin/create_bazel_cache_rcs.sh
fi


# runs custom docker data root cleanup binary and debugs remaining resources
cleanup_dind() {
    if [[ "${DOCKER_IN_DOCKER_ENABLED:-false}" == "true" ]]; then
        echo "Cleaning up after docker"
        docker ps -aq | xargs -r docker rm -f || true
        service docker stop || true
    fi
}

early_exit_handler() {
    cleanup_dind
}

# optionally enable ipv6 docker
export DOCKER_IN_DOCKER_IPV6_ENABLED=${DOCKER_IN_DOCKER_IPV6_ENABLED:-false}
if [[ "${DOCKER_IN_DOCKER_IPV6_ENABLED}" == "true" ]]; then
    echo "Enabling IPV6 for Docker."
    # configure the daemon with ipv6
    mkdir -p /etc/docker/
    cat <<EOF >/etc/docker/daemon.json
{
  "ipv6": true,
  "fixed-cidr-v6": "fc00:db8:1::/64"
}
EOF
    # enable ipv6
    sysctl net.ipv6.conf.all.disable_ipv6=0
    sysctl net.ipv6.conf.all.forwarding=1
    # enable ipv6 iptables
    modprobe -v ip6table_nat
fi

# Check if the job has opted-in to docker-in-docker availability.
export DOCKER_IN_DOCKER_ENABLED=${DOCKER_IN_DOCKER_ENABLED:-false}
if [[ "${DOCKER_IN_DOCKER_ENABLED}" == "true" ]]; then
    echo "Docker in Docker enabled, initializing..."
    printf '=%.0s' {1..80}; echo
    # If we have opted in to docker in docker, start the docker daemon,
    service docker start
    # the service can be started but the docker socket not ready, wait for ready
    WAIT_N=0
    MAX_WAIT=5
    while true; do
        # docker ps -q should only work if the daemon is ready
        docker ps -q > /dev/null 2>&1 && break
        if [[ ${WAIT_N} -lt ${MAX_WAIT} ]]; then
            WAIT_N=$((WAIT_N+1))
            echo "Waiting for docker to be ready, sleeping for ${WAIT_N} seconds."
            sleep ${WAIT_N}
        else
            echo "Reached maximum attempts, not waiting any longer..."
            break
        fi
    done
    printf '=%.0s' {1..80}; echo
    echo "Done setting up docker in docker."
fi

trap early_exit_handler INT TERM

# disable error exit so we can run post-command cleanup
set +o errexit

# add $GOPATH/bin to $PATH
export PATH="${GOPATH}/bin:${PATH}"
mkdir -p "${GOPATH}/bin"
# Authenticate gcloud, allow failures
if [[ -n "${GOOGLE_APPLICATION_CREDENTIALS:-}" ]]; then
  gcloud auth activate-service-account --key-file="${GOOGLE_APPLICATION_CREDENTIALS}" || true
fi

# Use a reproducible build date based on the most recent git commit timestamp.
SOURCE_DATE_EPOCH=$(git log -1 --pretty=%ct || true)
export SOURCE_DATE_EPOCH

# actually start bootstrap and the job
set -o xtrace
"$@"
EXIT_VALUE=$?
set +o xtrace

# cleanup after job
if [[ "${DOCKER_IN_DOCKER_ENABLED}" == "true" ]]; then
    echo "Cleaning up after docker in docker."
    printf '=%.0s' {1..80}; echo
    cleanup_dind
    printf '=%.0s' {1..80}; echo
    echo "Done cleaning up after docker in docker."
fi

# preserve exit value from job / bootstrap
exit ${EXIT_VALUE}
