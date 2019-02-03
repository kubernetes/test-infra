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

# add env we can debug with the image name:tag
ARG IMAGE_ARG
ENV IMAGE=${IMAGE_ARG}

ENV DEP_VER=v0.5.0
ENV DEP_CHECKSUM=287b08291e14f1fae8ba44374b26a2b12eb941af3497ed0ca649253e21ba2f83

RUN wget https://github.com/golang/dep/releases/download/${DEP_VER}/dep-linux-amd64 && \
    bash -c "sha256sum -c <(echo ${DEP_CHECKSUM} dep-linux-amd64)" && \
    mv dep-linux-amd64 /usr/local/bin/dep && \
    chmod +x /usr/local/bin/dep

ENV PATH "/etcd:${PATH}"
