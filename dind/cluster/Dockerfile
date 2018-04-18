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

FROM gcr.io/k8s-testimages/dind-test-base-amd64:0.2

# Get our short startup scripts first, because they're least likely to change.
COPY init-wrapper.sh /init-wrapper.sh
COPY start.sh /start.sh

# Get the debs
COPY kubectl.deb /

# Grab a file with the version info.
COPY docker_version /
COPY source_version /

# Install the binaries before getting the component images, because if they've
# changed, we're already redoing everything.

# kubeadm depends on kubelet depends on kubernetes-cni
RUN dpkg -i kubectl.deb

# Get the dind-node image.
COPY dind-node-bundle.tar /

# Get the test artifacts.
COPY e2e.test /
COPY ginkgo /
COPY dind-test.sh /
COPY cluster-up /

ENTRYPOINT ["/init-wrapper.sh"]
