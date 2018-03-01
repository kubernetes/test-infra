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

FROM docker.io/openshift/origin:latest
LABEL maintainer="dthau@redhat.com"

ENV GIT_COMMITTER_NAME=developer \
    GIT_COMMITTER_EMAIL=developer@redhat.com

RUN yum install make

COPY config-updater /config-updater
ENTRYPOINT ["/config-updater"]
