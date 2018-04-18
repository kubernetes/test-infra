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

FROM debian:jessie
LABEL maintainer="Benjamin Elder <bentheelder@google.com>"

# Includes everything needed to build and test github.com/kubernetes/kubernetes
# and github.com/kubernetes/test-infra with bazel and run bazel as the host UID

ARG BAZEL_VERSION

WORKDIR "/workspace"
RUN mkdir -p "/workspace"

# TODO(bentheelder): we should probably have these pip deps in bazel,
# remove these from the container if kettle is fixed.
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    ca-certificates \
    git \
    python \
    python-pip \
    rpm \
    unzip \
    wget \
    zip \
    zlib1g-dev \
    && apt-get clean && \
    python -m pip install --upgrade pip setuptools wheel && \
    pip install pylint pyyaml

SHELL ["/bin/bash", "-c"]

RUN INSTALLER="bazel-${BAZEL_VERSION}-installer-linux-x86_64.sh"; \
    if [[ "${BAZEL_VERSION}" =~ ([0-9\.]+)(rc[0-9]+) ]]; then \
      DOWNLOAD_URL="https://storage.googleapis.com/bazel/${BASH_REMATCH[1]}/${BASH_REMATCH[2]}/${INSTALLER}"; \
    else \
      DOWNLOAD_URL="https://github.com/bazelbuild/bazel/releases/download/${BAZEL_VERSION}/${INSTALLER}"; \
    fi; \
    wget -q "${DOWNLOAD_URL}" && \
    chmod +x "${INSTALLER}" && "./${INSTALLER}" && rm "${INSTALLER}"

# It turns out git and bazel pkg_tar and a bunch of other things fail if we
# don't have a /etc/passwd with the user in it, so let's solve that.
# We're making a fake world-writable one *inside* the container so that the
# entrypoint can write an entry to it with the user matching the host user
# this means we don't need to run the container as root! :-)
# We care about not running as root because it means build files are owned by
# the host users, not root (and logs, etc).
RUN touch /etc/passwd && chmod a+rw /etc/passwd

COPY ./entrypoint.sh /
RUN chmod +rx /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
