# Copyright 2019 The Kubernetes Authors.
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

# Includes tools used for sigs.k8s.io/kind CI
# NOTE: we attempt to avoid unnecessary tools and image layers while
# supporting kubernetes builds, kind installation, etc.

# arg that specifies the image name (for debugging)
ARG IMAGE_ARG
# arg that specifies the go version to install
ARG GO_VERSION

# add envs:
# - so we can debug with the image name:tag
# - adding gsutil etc. to path (where we will install them)
# - disabling prompts when installing gsutil etc.
# - hinting that we are in a docker container
ENV KRTE_IMAGE=${IMAGE_ARG} \
    BAZEL_VERSION=${BAZEL_VERSION_ARG} \
    GOPATH=/home/prow/go \
    PATH=/home/prow/go/bin:/usr/local/go/bin:/google-cloud-sdk/bin:${PATH} \
    CLOUDSDK_CORE_DISABLE_PROMPTS=1 \
    CONTAINER=docker

# copy in image utility scripts
COPY wrapper.sh /usr/local/bin/

# Install tools needed to:
# - install docker
# - build kind (dockerized)
# - build kubernetes (dockerized, or with bazel)
#
# TODO: the `sed` is a bit of a hack, look into alternatives.
# Why this exists: `docker service start` on debian runs a `cgroupfs_mount` method,
# We're already inside docker though so we can be sure these are already mounted.
# Trying to remount these makes for a very noisy error block in the beginning of
# the pod logs, so we just comment out the call to it... :shrug:
RUN echo "Installing Packages ..." \
        && apt-get update \
        && apt-get install -y --no-install-recommends \
            apt-transport-https \
            build-essential \
            ca-certificates \
            curl \
            file \
            git \
            gnupg2 \
            kmod \
            lsb-release \
            mercurial \
            pkg-config \
            procps \
            python \
            python-dev \
            python-pip \
            rsync \
            software-properties-common \
            unzip \
        && rm -rf /var/lib/apt/lists/* \
    && echo "Installing Go ..." \
        && export GO_TARBALL="go${GO_VERSION}.linux-amd64.tar.gz"\
        && curl -fsSL "https://storage.googleapis.com/golang/${GO_TARBALL}" --output "${GO_TARBALL}" \
        && tar xzf "${GO_TARBALL}" -C /usr/local \
        && rm "${GO_TARBALL}"\
        && mkdir -p "${GOPATH}/bin" \
    && echo "Installing gcloud SDK, kubectl ..." \
        && curl -fsSL https://dl.google.com/dl/cloudsdk/channels/rapid/google-cloud-sdk.tar.gz --output google-cloud-sdk.tar.gz \
        && tar xzf google-cloud-sdk.tar.gz -C / \
        && rm google-cloud-sdk.tar.gz \
        && /google-cloud-sdk/install.sh \
            --disable-installation-options \
            --bash-completion=false \
            --path-update=false \
            --usage-reporting=false \
        && gcloud components install kubectl \
    && echo "Installing Docker ..." \
        && curl -fsSL https://download.docker.com/linux/$(. /etc/os-release; echo "$ID")/gpg | apt-key add - \
        && add-apt-repository \
            "deb [arch=amd64] https://download.docker.com/linux/$(. /etc/os-release; echo "$ID") \
            $(lsb_release -cs) stable" \
        && apt-get update \
        && apt-get install -y --no-install-recommends docker-ce \
        && rm -rf /var/lib/apt/lists/* \
        && sed -i 's/cgroupfs_mount$/#cgroupfs_mount\n/' /etc/init.d/docker \
    && echo "Ensuring Legacy Iptables ..." \
        && update-alternatives --set iptables /usr/sbin/iptables-legacy \
        && update-alternatives --set ip6tables /usr/sbin/ip6tables-legacy

# entrypoint is our wrapper script, in Prow you will need to explicitly re-specify this
ENTRYPOINT ["wrapper.sh"]
# volume for docker in docker, use an emptyDir in Prow
VOLUME ["/var/lib/docker"]
