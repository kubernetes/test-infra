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

FROM debian:bookworm

# arg that specifies the image name (for debugging)
ARG IMAGE_ARG

# arg that specifies the go version to install.
# empty value specifies the latest version.
ARG GO_VERSION

# args for the user to run rootless docker (for KRTE_SYSTEMD_ROOTLESS mode)
ARG ROOTLESS_USER=rootless
ARG ROOTLESS_UID=5000
ARG ROOTLESS_SUBUID_BEGIN=100000
ARG ROOTLESS_SUBUID_COUNT=1048576

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

# Install tools needed to:
# - install docker
# - build kind
# - build kubernetes
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
            dbus \
            dbus-user-session \
            file \
            git \
            gnupg2 \
            iproute2 \
            kmod \
            lsb-release \
            mercurial \
            pkg-config \
            procps \
            python3 \
            rsync \
            slirp4netns \
            software-properties-common \
            sudo \
            systemd \
            uidmap \
            unzip \
        && rm -rf /var/lib/apt/lists/* \
    && echo "Installing Go ..." \
        && if [ -z "${GO_VERSION}" ]; then GO_VERSION=$(curl -fsSL https://go.dev/VERSION?m=text | grep -oP "go\K(.*)"); fi \
        && export GO_TARBALL="go${GO_VERSION}.linux-amd64.tar.gz" \
        && curl -fsSL "https://go.dev/dl/${GO_TARBALL}" --output "${GO_TARBALL}" \
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
        && curl -fsSL https://download.docker.com/linux/$(. /etc/os-release; echo "$ID")/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg \
        && chmod a+r /etc/apt/keyrings/docker.gpg \
        && echo "deb [arch="$(dpkg --print-architecture)" signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/debian \
            "$(. /etc/os-release && echo "$VERSION_CODENAME")" stable" | \
            tee /etc/apt/sources.list.d/docker.list > /dev/null \
        && apt-get update \
        && apt-get install -y --no-install-recommends docker-ce docker-buildx-plugin docker-ce-rootless-extras \
        && systemctl disable docker \
        && rm -rf /var/lib/apt/lists/* \
        && sed -i 's/cgroupfs_mount$/#cgroupfs_mount\n/' /etc/init.d/docker \
    && echo "Ensuring Legacy Iptables ..." \
        && update-alternatives --set iptables /usr/sbin/iptables-legacy \
        && update-alternatives --set ip6tables /usr/sbin/ip6tables-legacy \
    && echo "Setting up the systemd delegation for rootless" \
        && mkdir -p /etc/systemd/system/user@.service.d \
        && echo "[Service]" > /etc/systemd/system/user@.service.d/delegate.conf \
        && echo "Delegate=yes" >> /etc/systemd/system/user@.service.d/delegate.conf \
    && echo "Setting up the rootless user" \
        && useradd --create-home --home-dir /home/${ROOTLESS_USER} --uid ${ROOTLESS_UID} -G systemd-journal ${ROOTLESS_USER} \
        && echo "${ROOTLESS_UID}:${ROOTLESS_SUBUID_BEGIN}:${ROOTLESS_SUBUID_COUNT}" >/etc/subuid \
        && cp -f /etc/subuid /etc/subgid \
        && mkdir -p /home/${ROOTLESS_USER}/.local/share/docker \
        && chown -R ${ROOTLESS_USER} /home/${ROOTLESS_USER}

# copy in image utility scripts
COPY containerized-systemd.sh /usr/local/bin/
COPY wrapper.sh /usr/local/bin/

# entrypoint is our wrapper script, in Prow you will need to explicitly re-specify this
ENTRYPOINT ["wrapper.sh"]
# volume for docker in docker, use an emptyDir in Prow
VOLUME ["/var/lib/docker"]
VOLUME ["/home/${ROOTLESS_USER}/.local/share/docker"]
