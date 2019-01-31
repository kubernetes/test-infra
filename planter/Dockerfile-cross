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

ARG BASEIMAGE

FROM $BASEIMAGE

# Add the crossbuild-essential packages to the planter image.
# debian:stretch doesn't have the s390x package.
# Instead we download from Ubuntu, like kube-cross does.
RUN apt-get update && apt-get install -y gnupg \
    && echo "deb http://archive.ubuntu.com/ubuntu xenial main universe" > /etc/apt/sources.list.d/cgocrosscompiling.list \
    && apt-key adv --no-tty --keyserver keyserver.ubuntu.com --recv-keys 40976EAF437D05B5 3B4FE6ACC0B21F32 \
    && apt-get update \
    && apt-get install -y --no-install-recommends \
    crossbuild-essential-armhf \
    crossbuild-essential-arm64 \
    crossbuild-essential-ppc64el \
    crossbuild-essential-s390x \
    && rm -rf /var/lib/apt/lists/*
