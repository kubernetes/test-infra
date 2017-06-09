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

FROM gcr.io/k8s-testimages/kubekins-e2e:v20170605-ed5d94ed
MAINTAINER beacham@google.com

RUN apt-get update && apt-get install -y \
    build-essential \
    pkg-config \
    unzip \
    bash-completion \
    git \
    wget && \
    apt-get clean

ENV TERRAFORM_VERSION=0.7.2 \
    KUBERNETES_PROVIDER=kubernetes-anywhere \
    KUBECTL_VERSION=1.6.4 \
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

# TODO(pipejakob): Instead of fetching a specific precompiled version, get
# kubernetes-anywhere to use the kubectl built as part of this job.
RUN wget https://storage.googleapis.com/kubernetes-release/release/v${KUBECTL_VERSION}/bin/linux/amd64/kubectl -O /usr/local/bin/kubectl && \
    chmod +x /usr/local/bin/kubectl

WORKDIR /workspace
ADD runner /
ENTRYPOINT ["/bin/bash", "/runner"]
