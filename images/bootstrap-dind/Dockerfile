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

# This file creates a build environment for building and running kubernetes
# unit and integration tests

ARG BOOTSTRAP_TAG
FROM gcr.io/k8s-testimages/bootstrap:${BOOTSTRAP_TAG}
LABEL maintainer="Benjamin Elder <bentheelder@google.com>"

#
# BEGIN: DOCKER IN DOCKER SETUP
#

# docker deps, some of these are already installed in the base image but
# that's fine since they won't re-install and we can reuse the code below
# for another image someday
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        apt-transport-https \
        ca-certificates \
        curl \
        gnupg2 \
        software-properties-common \
        lsb-release

RUN curl -fsSL https://download.docker.com/linux/$(. /etc/os-release; echo "$ID")/gpg | apt-key add -

RUN add-apt-repository \
    "deb [arch=amd64] https://download.docker.com/linux/$(. /etc/os-release; echo "$ID") \
    $(lsb_release -cs) \
    stable"

RUN apt-get update && \
    apt-get install -y --no-install-recommends docker-ce

# Move Docker's storage location
RUN echo 'DOCKER_OPTS="${DOCKER_OPTS} --graph=/docker-graph"' | \
        tee --append /etc/default/docker
# NOTE this should be mounted in from the host ideally (!)
# We will make a fake one now just in case
RUN mkdir /docker-graph

# Add a runner that wraps the original runner with a script that also runs:
# `service docker start`
# before running the original runner.
ADD ["dind-runner", \
    "/workspace/"]

ENTRYPOINT ["/workspace/dind-runner"]

#
# END: DOCKER IN DOCKER SETUP
#
