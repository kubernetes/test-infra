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
        digest = "sha256:bd327018b3effc802514b63cc90102bfcd92765f4486fc5abc28abf7eb9f1e4d",  # 2018/09/20
        registry = "gcr.io",
        repository = "k8s-prow/alpine",
        tag = "0.1",  # TODO(fejta): update or replace
    )

    container_pull(
        name = "alpine-bash",
        digest = "sha256:d520f733f3d648b81201b28b0f9894ad2940972c516e554958d0177470c6a881",  # 2019/07/29
        registry = "gcr.io",
        repository = "k8s-testimages/alpine-bash",
        tag = "latest",  # TODO(fejta): update or replace
    )

    container_pull(
        name = "boskosctl-base",
        digest = "sha256:a23c19a87857140926184d19e8e54812ba4a8acec4097386ca0993a248e83f8b",  # 2019/08/05
        registry = "gcr.io",
        repository = "k8s-testimages/boskosctl-base",
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
        digest = "sha256:01b0f83fe91b782ec7ddf1e742ab7cc9a2261894fd9ab0760ebfd39af2d6ab28",  # 2018/07/02
        registry = "gcr.io",
        repository = "k8s-prow/git",
        tag = "0.2",  # TODO(fejta): update or replace
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
        digest = "sha256:cefc822f93bb3dcf272ce3e4c5162b179d5c165584ace13856afed99662b87cd",
        registry = "gcr.io",
        repository = "k8s-testimages/launcher.gcr.io/google/bazel",
        tag = "2.2.0-from-2.0.0",  # TODO(fejta): switch to test-infra tag once it exists
    )

    container_pull(
        name = "cloud-sdk-slim",
        digest = "sha256:6dafecdad80abf6470eae9e0b57fc083d1f3413fa15b9fab7c2ad3a102d244c4",  # 2020/01/21
        registry = "gcr.io",
        repository = "google.com/cloudsdktool/cloud-sdk",
        tag = "277.0.0-slim",
    )
