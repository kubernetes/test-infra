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
        digest = "sha256:2b0a8e9a13dcc168b126778d9e947a7081b4d2ee1ee122830d835f176d0e2a70",
        registry = "gcr.io",
        repository = "distroless/base",
        # tag = "latest", as of Jul 15 2020
    )

    container_pull(
        name = "alpine-base",
        digest = "sha256:94eabd0927065a4fd03136179c4467fc42d3d08f78fd571e395599ff8521c210",
        registry = "gcr.io",
        repository = "k8s-prow/alpine",
        # tag = "v20200713-e9b3d9d",
    )

    container_pull(
        name = "gcloud-base",
        digest = "sha256:5b49dfb5e366dd75a5fc6d5d447be584f8f229c5a790ee0c3b0bd0cf70ec41dd",
        registry = "gcr.io",
        repository = "cloud-builders/gcloud",
        # tag = "latest",
    )

    container_pull(
        name = "git-base",
        digest = "sha256:d04b737f2a655f0a432ca7da64fde774292a313a96510b3a95ec8caacd8092cf",
        registry = "gcr.io",
        repository = "k8s-prow/git",
        # tag = "v20211018-372b1c8",
    )

    container_pull(
        name = "git-base-arm64",
        digest = "sha256:6ef92a6977693612922628be8d412e381fac58cc4918ff27679f73d84538d953",
        registry = "gcr.io",
        repository = "k8s-prow/git",
        # tag = "v20211018-372b1c8",
    )

    container_pull(
        name = "git-base-ppc64le",
        digest = "sha256:9333361f7f10d0f116dd6a3a39f38bf89e208ebf873b07ec0171772675d242ca",
        registry = "gcr.io",
        repository = "k8s-prow/git",
        # tag = "v20211018-372b1c8",
    )

    container_pull(
        name = "git-base-s390x",
        digest = "sha256:22617c9314ffae594db048083f094a1ee7d2da1eb98c414e741b701f3ac7ec48",
        registry = "gcr.io",
        repository = "k8s-prow/git",
        # tag = "v20211018-372b1c8",
    )

    container_pull(
        name = "bazel-base",
        digest = "sha256:e006f1c3658dd11d5176d5f7a862df4d5b9c06cfe014d5b8a86bb64b20a6f8be",
        registry = "gcr.io",
        repository = "k8s-testimages/launcher.gcr.io/google/bazel",
        tag = "v20210128-721ee66-test-infra",
    )
