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

# This file builds an image for the log exporter tool. For more info,
# have a look at the tool's README.md file.

FROM debian:stretch-20201209-slim
# systemd is needed as journalctl is used to fetch logs from
# k8s components that are run on nodes as systemd services.
RUN apt-get update && \
    apt-get install -y systemd python3

# Setup gcloud SDK for using gsutil.
ADD ["https://dl.google.com/dl/cloudsdk/channels/rapid/google-cloud-sdk.tar.gz", \
     "/workspace/"]
ENV PATH=/google-cloud-sdk/bin:/workspace:${PATH} \
    CLOUDSDK_CORE_DISABLE_PROMPTS=1
RUN tar xzf /workspace/google-cloud-sdk.tar.gz -C / && \
    /google-cloud-sdk/install.sh \
        --disable-installation-options \
        --bash-completion=false \
        --path-update=false \
        --usage-reporting=false && \
    gcloud info | tee /workspace/gcloud-info.txt

# Setup the log exporter script.
ADD ["logexporter", "/workspace/"]
WORKDIR "/workspace"

ENTRYPOINT ["/workspace/logexporter"]
