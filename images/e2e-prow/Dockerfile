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

FROM gcr.io/k8s-testimages/kubekins-e2e:v20170718-3867aab5
MAINTAINER  Sen Lu <senlu@google.com>

ADD runner /
ENTRYPOINT ["/bin/bash", "/runner"]
