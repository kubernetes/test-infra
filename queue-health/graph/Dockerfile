# Copyright 2016 The Kubernetes Authors All rights reserved.
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

FROM gcr.io/google-containers/queue-health-base:v20160627
MAINTAINER Erick Fejta <fejta@google.com>

# Install matplotlib
RUN apt-get update \
    && apt-get install -y \
         python-matplotlib

# Install grapher
COPY graph.py /

# When not running inside GCE ensure you copy over credentials:
# cp ~/.boto /tmp/foo/ && chmod 644 /tmp/foo/.boto
# docker run -v /tmp/foo:/boto -e BOTO_CONFIG=/boto/.boto derived-image
