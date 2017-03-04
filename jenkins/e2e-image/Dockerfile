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

FROM gcr.io/google-containers/kubekins-test:v20160822
MAINTAINER  Erick Fejta <fejta@google.com>

# Defaults of all e2e runs
ENV E2E_UP=true \
    E2E_TEST=true \
    E2E_DOWN=true

# Customize these as appropriate
ENV E2E_PUBLISH_PATH= \
    INSTANCE_PREFIX=jenkins-e2e \
    KUBERNETES_PROVIDER=gce

# Variables specific to GCP
ENV FAIL_ON_GCP_RESOURCE_LEAK=true \
    JOB_NAME=kubernetes-e2e-gce-conformance \
    KUBE_GCE_INSTANCE_PREFIX=jenkins-e2e \
    KUBE_GCE_NETWORK=jenkins-e2e \
    KUBE_GCE_ZONE=us-central1-f

# Variable specific to the machine:
# GOOGLE_APPLICATION_CREDENTIALS
# JENKINS_GCE_SSH_PRIVATE_KEY_FILE
# JENKINS_GCE_SSH_PUBLIC_KEY_FILE
# JENKINS_AWS_SSH_PRIVATE_KEY_FILE
# JENKINS_AWS_SSH_PUBLIC_KEY_FILE
# JENKINS_AWS_CREDENTIALS_FILE

ADD ["https://dl.google.com/dl/cloudsdk/channels/rapid/google-cloud-sdk.tar.gz", \
     "/workspace/"]
ENV PATH=/google-cloud-sdk/bin:${PATH} \
    CLOUDSDK_CORE_DISABLE_PROMPTS=1
RUN tar xzf /workspace/google-cloud-sdk.tar.gz -C / && \
    /google-cloud-sdk/install.sh \
        --disable-installation-options \
        --bash-completion=false \
        --path-update=false \
        --usage-reporting=false && \
    gcloud components install beta && \
    gcloud components install alpha && \
    gcloud info | tee /workspace/gcloud-info.txt

ADD ["e2e-runner.sh", \
    "kops-e2e-runner.sh", \
    "kubetest", \
    "https://raw.githubusercontent.com/kubernetes/kubernetes/master/cluster/get-kube.sh", \
    "kubetest", \
    "/workspace/"]
RUN ["chmod", "+x", "/workspace/get-kube.sh"]
WORKDIR "/workspace"

ENTRYPOINT ["/workspace/e2e-runner.sh"]
