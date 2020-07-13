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

load("@io_bazel_rules_docker//container:container.bzl", "container_pull")

def repositories():
    container_pull(
        name = "distroless-base",
        digest = "sha256:e37cf3289c1332c5123cbf419a1657c8dad0811f2f8572433b668e13747718f8",
        registry = "gcr.io",
        repository = "distroless/base",
        tag = "latest",
    )

    container_pull(
        name = "alpine-base",
        digest = "sha256:55a0c44a09ede57e3193fae6d0918865f2c4d7effe7a8f4dad72eef31f6f7841",
        registry = "gcr.io",
        repository = "k8s-prow/alpine",
        # tag = "v20200605-44f6c96",
    )

    container_pull(
        name = "alpine-bash",
        digest = "sha256:d520f733f3d648b81201b28b0f9894ad2940972c516e554958d0177470c6a881",  # 2019/07/29
        registry = "gcr.io",
        repository = "k8s-testimages/alpine-bash",
        tag = "latest",  # TODO(fejta): update or replace
    )

    container_pull(
        name = "gcloud-base",
        digest = "sha256:8e51eea50a45c6be2a735be97139f85a04c623ca448801a317a737c1d9917d00",  # 2019/08/16
        registry = "gcr.io",
        repository = "cloud-builders/gcloud",
        tag = "latest",
    )

    container_pull(
        name = "git-base",
        digest = "sha256:45a5255060b34151ec1fa913eb0bc18958c909fc088989dd84f0614a22fb1840",
        registry = "gcr.io",
        repository = "k8s-prow/git",
        # tag = "v20200605-44f6c96",
    )

    container_pull(
        name = "python",
        digest = "sha256:594a43a1eb22f5a37b15e0394fc0e39e444072e413f10a60bac0babe42280304",  # 2019/08/16
        registry = "index.docker.io",
        repository = "library/python",
        tag = "2",
    )

    container_pull(
        name = "gcloud-go",
        digest = "sha256:0dd11e500c64b7e722ad13bc9616598a14bb0f66d9e1de4330456c646eaf237d",  # 2019/01/25
        registry = "gcr.io",
        repository = "k8s-testimages/gcloud-in-go",
        tag = "v20190125-cc5d6ecff3",  # TODO(fejta): update or replace
    )

    container_pull(
        name = "bazel-base",
        digest = "sha256:2e8163b61f3759f6ff0e4df43c40d092dae331b1c2d5326f05f78e72a68d3203",
        registry = "gcr.io",
        repository = "k8s-testimages/launcher.gcr.io/google/bazel",
        tag = "v20200609-e7bfd25-test-infra",
    )
