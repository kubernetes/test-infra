# Copyright 2021 The Kubernetes Authors.
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

FROM docker:latest

ARG BUILDX_VERSION=0.5.1

ENV DOCKER_BUILDKIT=1
ENV DOCKER_CLI_EXPERIMENTAL=enabled

RUN mkdir -p $HOME/.docker/cli-plugins && \
    wget -O $HOME/.docker/cli-plugins/docker-buildx https://github.com/docker/buildx/releases/download/v${BUILDX_VERSION}/buildx-v${BUILDX_VERSION}.linux-amd64 && \
    chmod a+x $HOME/.docker/cli-plugins/docker-buildx

COPY entrypoint.sh /usr/local/bin/

ENTRYPOINT ["entrypoint.sh"]
