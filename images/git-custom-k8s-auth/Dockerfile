# Copyright 2022 The Kubernetes Authors.
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

FROM google/cloud-sdk:latest as builder
# https://github.com/GoogleCloudPlatform/cloud-sdk-docker/blob/master/Dockerfile

ARG AWS_IAM_AUTHENTICATOR_VERSION
RUN curl -fsSL \
    https://github.com/kubernetes-sigs/aws-iam-authenticator/releases/download/v${AWS_IAM_AUTHENTICATOR_VERSION}/aws-iam-authenticator_${AWS_IAM_AUTHENTICATOR_VERSION}_linux_amd64 -o /aws-iam-authenticator && \
    chmod +x /aws-iam-authenticator


FROM alpine:latest

ARG IMAGE_ARG
ENV IMAGE=${IMAGE_ARG}

RUN apk add --no-cache ca-certificates git git-daemon openssh && update-ca-certificates
COPY --from=builder /aws-iam-authenticator /usr/local/bin/aws-iam-authenticator
COPY --from=builder /usr/bin/gke-gcloud-auth-plugin /usr/local/bin/gke-gcloud-auth-plugin
COPY github-known-hosts /github_known_hosts
COPY ssh-config /etc/ssh/ssh_config
RUN aws-iam-authenticator version && gke-gcloud-auth-plugin --version
