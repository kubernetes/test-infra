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

FROM jamiehewland/alpine-pypy:3

# Google Cloud SDK requires python 2.7.9+ (https://cloud.google.com/python/setup#installing_the_cloud_sdk)
RUN apk add --no-cache jq curl bash python2
RUN curl -o installer https://sdk.cloud.google.com && bash installer --disable-prompts --install-dir=/ && rm installer && ln -s /google-cloud-sdk/bin/* /bin/

ADD *.py update_summaries.sh /

# Point GOOGLE_APPLICATION_CREDENTIALS at a serviceaccount.json with the necessary permissions.
ENTRYPOINT ["timeout", "10800", "/update_summaries.sh"]
