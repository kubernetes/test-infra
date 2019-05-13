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

# See rbe_autoconfig() in rbe_repo.bzl (digest comes from versions.bzl)
# https://github.com/bazelbuild/bazel-toolchains/blob/master/rules/rbe_repo.bzl
# https://github.com/bazelbuild/bazel-toolchains/blob/master/configs/ubuntu16_04_clang/versions.bzl
FROM marketplace.gcr.io/google/rbe-ubuntu16-04@sha256:677c1317f14c6fd5eba2fd8ec645bfdc5119f64b3e5e944e13c89e0525cc8ad1

# add env we can debug with the image name:tag
ARG IMAGE_ARG
ENV IMAGE=${IMAGE_ARG}

RUN apt-get update && apt-get install -y --no-install-recommends \
   rpm
