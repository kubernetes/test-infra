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

# This image is also constructed with Bazel,
# please make sure this matches the gcloud-go entry in WORKSPACE
FROM gcr.io/k8s-testimages/gcloud-in-go:v20190125-cc5d6ecff3

RUN apt-get install ca-certificates && update-ca-certificates && \
    rm -rf /var/lib/apt/lists/*

COPY aws-janitor /aws-janitor
ENTRYPOINT ["/bin/bash", "/runner"]
