# Copyright 2015 The Kubernetes Authors. All rights reserved.
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

FROM gcr.io/google_containers/ubuntu-slim:0.4

RUN apt-get update && apt-get install -y --no-install-recommends bash

COPY build.sh /build.sh

ENV VERSION 1.2.23
ENV SHA256 43a321964bd83a9080ee2d486f8cd1fa47dd29f9c4e6411c2ec39bd81ec2076f

RUN /build.sh
