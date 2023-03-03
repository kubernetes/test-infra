# Copyright 2021 The Kubernetes Authors.
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

FROM gcr.io/k8s-staging-test-infra/bootstrap:v20210913-fc7c4e84f6

FROM gcr.io/oss-fuzz-base/cifuzz-base:latest
COPY --from=gcr.io/oss-fuzz-base/cifuzz-base:latest /opt/oss-fuzz /opt/

#
# BEGIN: DOCKER IN DOCKER SETUP
# 
 
# Install Docker deps, some of these are already installed in the image but
# that's fine since they won't re-install and we can reuse the code below
# for another image someday.
RUN apt-get update && apt-get install -y --no-install-recommends \
    apt-transport-https \
    ca-certificates \
    curl \
    gnupg2 \
    software-properties-common \
    lsb-release && \
    rm -rf /var/lib/apt/lists/*
 
# Add the Docker apt-repository
RUN curl -fsSL https://download.docker.com/linux/$(. /etc/os-release; echo "$ID")/gpg \
    | apt-key add - && \
    add-apt-repository \
    "deb [arch=amd64] https://download.docker.com/linux/$(. /etc/os-release; echo "$ID") \
    $(lsb_release -cs) stable"
 
# Install Docker
# TODO: the `sed` is a bit of a hack, look into alternatives.
# Why this exists: `docker service start` on debian runs a `cgroupfs_mount` method,
# We're already inside docker though so we can be sure these are already mounted.
# Trying to remount these makes for a very noisy error block in the beginning of
# the pod logs, so we just comment out the call to it... :shrug:
RUN apt-get update && \
    apt-get install -y --no-install-recommends docker-ce=5:20.10.* && \
    rm -rf /var/lib/apt/lists/* && \
    sed -i 's/cgroupfs_mount$/#cgroupfs_mount\n/' /etc/init.d/docker \
    && update-alternatives --set iptables /usr/sbin/iptables-legacy \
    && update-alternatives --set ip6tables /usr/sbin/ip6tables-legacy

# Move Docker's storage location
RUN echo 'DOCKER_OPTS="${DOCKER_OPTS} --data-root=/docker-graph"' | \
    tee --append /etc/default/docker
 
#
# END: DOCKER IN DOCKER SETUP
#

COPY --from=gcr.io/k8s-staging-test-infra/bootstrap:v20210913-fc7c4e84f6 /usr/local/bin/runner.sh /usr/local/bin/runner.sh
COPY --from=gcr.io/k8s-staging-test-infra/bootstrap:v20210913-fc7c4e84f6 /usr/local/bin/entrypoint.sh /usr/local/bin/entrypoint.sh

ENTRYPOINT runner.sh "python3"  "/opt/oss-fuzz/infra/cifuzz/cifuzz_combined_entrypoint.py"
