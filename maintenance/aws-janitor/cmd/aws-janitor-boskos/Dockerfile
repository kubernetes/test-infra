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

FROM debian:stretch-20190204-slim

RUN apt-get -y update && \
    apt-get -y install ca-certificates && \
    update-ca-certificates && \
    rm -rf /var/lib/apt/lists/*

COPY aws-janitor-boskos /usr/local/bin/

ENTRYPOINT ["/bin/sh", "-c", "/bin/echo started && /usr/local/bin/aws-janitor-boskos \"$@\"", "--"]
