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

FROM gcr.io/k8s-testimages/kubekins-e2e:v20181203-a441fa430-master
LABEL maintainer "beacham@google.com"

# add env we can debug with the image name:tag
ARG IMAGE_ARG
ENV IMAGE=${IMAGE_ARG}

RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    pkg-config \
    unzip \
    bash-completion \
    git \
    wget && \
    apt-get clean

ENV TERRAFORM_VERSION=0.9.4 \
    KUBERNETES_PROVIDER=kubernetes-anywhere \
    PATH=/usr/local/bin:${PATH}

# Add kubernetes-anywhere dependencies: jsonnet, terraform, kubectl
RUN cd /tmp && \
    git clone https://github.com/google/jsonnet.git && \
    ( cd jsonnet && \
      make jsonnet && \
      cp jsonnet /bin \
    ) && \
    rm -rf /tmp/jsonnet

RUN mkdir -p /tmp/terraform/ && \
    ( cd /tmp/terraform && \
      wget https://releases.hashicorp.com/terraform/${TERRAFORM_VERSION}/terraform_${TERRAFORM_VERSION}_linux_amd64.zip && \
      unzip terraform_${TERRAFORM_VERSION}_linux_amd64.zip -d /bin \
    ) && \
    rm -rf /tmp/terraform

WORKDIR /workspace
