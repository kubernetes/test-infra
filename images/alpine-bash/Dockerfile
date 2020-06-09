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

# Includes bash, see //containers.bzl
FROM gcr.io/k8s-prow/alpine:v20200605-44f6c96

# add env we can debug with the image name:tag
ARG IMAGE_ARG
ENV IMAGE=${IMAGE_ARG}

RUN apk add --no-cache bash

ENTRYPOINT ["/bin/bash"]
