# Copyright 2024 The Kubernetes Authors.
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

# Includes basic workspace setup, with gcloud and a bootstrap runner

FROM debian:bookworm
ARG TARGETARCH

WORKDIR /workspace
RUN mkdir -p /workspace
ENV WORKSPACE=/workspace \
    TERM=xterm

# add env we can debug with the image name:tag
ARG IMAGE_ARG
ENV IMAGE=${IMAGE_ARG}

# common util tools
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    ca-certificates \
    curl \
    file \
    git \
    iproute2 \
    iputils-ping \
    jq \
    kmod \
    mercurial \
    openssh-client \
    pkg-config \
    procps \
    python3 \
    python3-distutils \
    python3-gflags \
    python3-pip \
    python3-venv \
    python3-yaml \
    rsync \
    unzip \
    wget \
    xz-utils \
    zip \
    zlib1g-dev \
    graphviz \
    bc \
    && rm -rf /var/lib/apt/lists/* \
    && python3 -m pip install --no-cache-dir --break-system-packages --upgrade pip setuptools wheel

# Install gcloud

ENV PATH=/google-cloud-sdk/bin:/workspace:${PATH} \
    CLOUDSDK_CORE_DISABLE_PROMPTS=1

ARG GCLOUD_SDK_URL=https://dl.google.com/dl/cloudsdk/channels/rapid/google-cloud-sdk.tar.gz
RUN wget -O google-cloud-sdk.tar.gz -q $GCLOUD_SDK_URL && \
    tar xzf google-cloud-sdk.tar.gz -C / && \
    rm google-cloud-sdk.tar.gz && \
    /google-cloud-sdk/install.sh \
    --disable-installation-options \
    --bash-completion=false \
    --path-update=false \
    --usage-reporting=false && \
    gcloud components install alpha beta kubectl && \
    gcloud info | tee /workspace/gcloud-info.txt


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
RUN curl -fsSL https://download.docker.com/linux/$(. /etc/os-release; echo "$ID")/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg \
    && chmod a+r /etc/apt/keyrings/docker.gpg \
    && echo "deb [arch="$(dpkg --print-architecture)" signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/debian \
    "$(. /etc/os-release && echo "$VERSION_CODENAME")" stable" | \
    tee /etc/apt/sources.list.d/docker.list > /dev/null

# Install Docker
# TODO: the `sed` is a bit of a hack, look into alternatives.
# Why this exists: `docker service start` on debian runs a `cgroupfs_mount` method,
# We're already inside docker though so we can be sure these are already mounted.
# Trying to remount these makes for a very noisy error block in the beginning of
# the pod logs, so we just comment out the call to it... :shrug:
RUN apt-get update && \
    apt-get install -y --no-install-recommends docker-ce docker-buildx-plugin && \
    rm -rf /var/lib/apt/lists/* && \
    sed -i 's/cgroupfs_mount$/#cgroupfs_mount\n/' /etc/init.d/docker \
    && update-alternatives --set iptables /usr/sbin/iptables-legacy \
    && update-alternatives --set ip6tables /usr/sbin/ip6tables-legacy



# Move Docker's storage location
RUN echo 'DOCKER_OPTS="${DOCKER_OPTS} --data-root=/docker-graph"' | \
    tee --append /etc/default/docker
# NOTE this should be mounted and persisted as a volume ideally (!)
# We will make a fallback one now just in case
RUN mkdir /docker-graph

#
# END: DOCKER IN DOCKER SETUP
#

# install cfssl to prevent https://github.com/kubernetes/kubernetes/issues/55589
# The invocation at the end is to prevent download failures downloads as in the bug.
# TODO(porridge): bump CFSSL_VERSION to one where cfssljson supports the -version flag and test it as well.
ARG CFSSL_VERSION
RUN wget -q -O cfssl "https://github.com/cloudflare/cfssl/releases/download/v${CFSSL_VERSION}/cfssl_${CFSSL_VERSION}_linux_${TARGETARCH}" && \
    wget -q -O cfssljson "https://github.com/cloudflare/cfssl/releases/download/v${CFSSL_VERSION}/cfssljson_${CFSSL_VERSION}_linux_${TARGETARCH}" && \
    chmod +x cfssl cfssljson && \
    mv cfssl cfssljson /usr/local/bin && \
    cfssl version

# replace kubectl with one from K8S_RELEASE
ARG K8S_RELEASE=latest
RUN rm -f $(which kubectl) && \
    export KUBECTL_VERSION=$(curl -L https://dl.k8s.io/release/${K8S_RELEASE}.txt) && \
    wget https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${TARGETARCH}/kubectl -O /usr/local/bin/kubectl && \
    chmod +x /usr/local/bin/kubectl

# install go
ARG GO_VERSION
ENV GO_TARBALL "go${GO_VERSION}.linux-${TARGETARCH}.tar.gz"
RUN wget -q "https://go.dev/dl/${GO_TARBALL}" && \
    tar xzf "${GO_TARBALL}" -C /usr/local && \
    rm "${GO_TARBALL}"

# install yq
ARG YQ_VERSION
RUN wget -q "https://github.com/mikefarah/yq/releases/download/${YQ_VERSION}/yq_linux_${TARGETARCH}" \
    -O /usr/local/bin/yq && chmod +x /usr/local/bin/yq

# install bazel
ARG BAZEL_VERSION_ARG
ENV BAZEL_VERSION=${BAZEL_VERSION_ARG}
COPY images/kubekins-e2e/install-bazel.sh /
RUN bash /install-bazel.sh

ARG OLD_BAZEL_VERSION
COPY --from=old \
    /usr/local/lib/bazel/bin/bazel-real /usr/local/lib/bazel/bin/bazel-${OLD_BAZEL_VERSION}

# install kind if a version is provided
ARG KIND_VERSION
RUN if [ -n "${KIND_VERSION}" ]; then \
    wget -q -O /usr/local/bin/kind https://kind.sigs.k8s.io/dl/v${KIND_VERSION}/kind-linux-${TARGETARCH} && \
    chmod +x /usr/local/bin/kind; \
    fi

# install kubetest2 binaries if a version is provided
# kubetest2 must be installed from git as the makefile embeds the git short hash and the day it was built.
ARG KUBETEST2_VERSION
RUN if [ -n "${KUBETEST2_VERSION}" ]; then \
    git clone https://github.com/kubernetes-sigs/kubetest2.git /tmp/kubetest2 && \
    cd /tmp/kubetest2 && make install-all && rm -rf /tmp/kubetest2; \
    fi

# configure dockerd to use mirror.gcr.io
# per instructions at https://cloud.google.com/container-registry/docs/pulling-cached-images
ARG DOCKER_REGISTRY_MIRROR_URL=https://mirror.gcr.io
RUN [ -n "${DOCKER_REGISTRY_MIRROR_URL}" ] && \
    echo "DOCKER_OPTS=\"\${DOCKER_OPTS} --registry-mirror=${DOCKER_REGISTRY_MIRROR_URL}\"" | \
    tee --append /etc/default/docker

# add env we can debug with the image name:tag
ARG IMAGE_ARG
ENV IMAGE=${IMAGE_ARG}


# note the runner is also responsible for making docker in docker function if
# env DOCKER_IN_DOCKER_ENABLED is set
COPY ["runner.sh", \
    "/usr/local/bin/"]

ENTRYPOINT ["/usr/local/bin/runner.sh"]
