# Copyright 2016 The Kubernetes Authors.
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

FROM gcr.io/k8s-testimages/bootstrap:v20181010-45051ef05
LABEL maintainer "Sen Lu <senlu@google.com>"

# hint to kubetest that it is in CI
ENV KUBETEST_IN_DOCKER="true"

# Go standard envs
ENV GOPATH /go
ENV PATH /usr/local/go/bin:$PATH
ENV PATH $GOPATH/bin:$PATH

# setup k8s repo symlink
RUN mkdir -p /go/src/k8s.io/kubernetes \
    && ln -s /go/src/k8s.io/kubernetes /workspace/kubernetes

# preinstall:
# - graphviz package for graphing profiles
# - bc for shell to junit
# - rpm for building RPMs with Bazel
RUN apt-get install -y bc \
    graphviz \
    rpm
# preinstall this for kops tests xref kubetest prepareAws(...)
# TODO(bentheelder,krzyzacy,justisb,chrislovecnm): remove this
RUN pip install awscli

# install cfssl to prevent https://github.com/kubernetes/kubernetes/issues/55589
# The invocation at the end is to prevent download failures downloads as in the bug.
# TODO(porridge): bump CFSSL_VERSION to one where cfssljson supports the -version flag and test it as well.
ARG CFSSL_VERSION
RUN wget -q -O cfssl "https://pkg.cfssl.org/${CFSSL_VERSION}/cfssl_linux-amd64" && \
    wget -q -O cfssljson "https://pkg.cfssl.org/${CFSSL_VERSION}/cfssljson_linux-amd64" && \
    chmod +x cfssl cfssljson && \
    mv cfssl cfssljson /usr/local/bin && \
    cfssl version

# install go
ARG GO_VERSION
ENV GO_TARBALL "go${GO_VERSION}.linux-amd64.tar.gz"
RUN wget -q "https://storage.googleapis.com/golang/${GO_TARBALL}" && \
    tar xzf "${GO_TARBALL}" -C /usr/local && \
    rm "${GO_TARBALL}"

# install bazel
ARG BAZEL_VERSION_ARG
ENV BAZEL_VERSION=${BAZEL_VERSION_ARG}
COPY ./install-bazel.sh /
RUN bash /install-bazel.sh

# if UPGRADE_DOCKER_ARG, then install the latest docker over whatever we have
# in the base image.
# TODO(bentheelder): after code freeze, roll out newer docker in the
# base image and possibly remove this ...
ARG UPGRADE_DOCKER_ARG=false
RUN [ "${UPGRADE_DOCKER_ARG}" = "true" ] && \
    apt-get install -y --no-install-recommends docker-ce && \
    sed -i 's/cgroupfs_mount$/#cgroupfs_mount\n/' /etc/init.d/docker || true

# add env we can debug with the image name:tag
ARG IMAGE_ARG
ENV IMAGE=${IMAGE_ARG}

# everything below will be triggered on every new image tag ...
ADD ["kops-e2e-runner.sh", \
    "kubetest", \
    "https://raw.githubusercontent.com/kubernetes/kubernetes/master/cluster/get-kube.sh", \
    "/workspace/"]
RUN ["chmod", "+x", "/workspace/get-kube.sh"]
