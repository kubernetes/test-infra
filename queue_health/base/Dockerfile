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

FROM ubuntu:14.04
LABEL maintainer="Erick Fejta <fejta@google.com>"

# Set gcloud environment variables
ENV CLOUDSDK_CORE_DISABLE_PROMPTS=1 \
    PATH="/google-cloud-sdk/bin:${PATH}"
# Install gcloud and python-requests
RUN apt-get update \
    && apt-get install -y \
	 python-requests \
	 wget \
    && wget https://dl.google.com/dl/cloudsdk/channels/rapid/google-cloud-sdk.tar.gz \
    && tar xzf google-cloud-sdk.tar.gz -C / \
    && /google-cloud-sdk/install.sh \
         --disable-installation-options \
	 --bash-completion=false \
	 --path-update=false \
	 --usage-reporting=false \
    && rm google-cloud-sdk.tar.gz

# When not running inside GCE ensure you copy over credentials:
# cp ~/.boto /tmp/foo/ && chmod 644 /tmp/foo/.boto
# docker run -v /tmp/foo:/boto -e BOTO_CONFIG=/boto/.boto derived-image
CMD wget -O - http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token \
    && echo gsutil configured \
    || echo please set -v /tmp/FOO:/boto -e BOTO_CONFIG=/boto/FOO.boto
