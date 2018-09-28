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

# Includes go, gcloud, and etcd
FROM gcr.io/k8s-testimages/gcloud-in-go:v20180927-6b4facbe6
LABEL maintainer="krousey@google.com"

# add env we can debug with the image name:tag
ARG IMAGE_ARG
ENV IMAGE=${IMAGE_ARG}

ENV ETCD_VER=v3.3.0
ENV ETCD_URL=https://storage.googleapis.com/etcd

RUN wget ${ETCD_URL}/${ETCD_VER}/etcd-${ETCD_VER}-linux-amd64.tar.gz && \
    mkdir /etcd && \
    tar xzf etcd-${ETCD_VER}-linux-amd64.tar.gz -C /etcd --strip-components=1 && \
    rm -f etcd-${ETCD_VER}-linux-amd64.tar.gz

ENV PATH "/etcd:${PATH}"
