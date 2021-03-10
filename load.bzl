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

load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")
load("@bazel_tools//tools/build_defs/repo:git.bzl", "git_repository", "new_git_repository")

def repositories():
    if not native.existing_rule("io_k8s_repo_infra"):
        http_archive(
            name = "io_k8s_repo_infra",
            sha256 = "2f30f87259fed7262d9b95b2665e3d3ecd928d174a4f0356063bc99056b6f84c",
            strip_prefix = "repo-infra-0.2.0",
            urls = [
                "https://github.com/kubernetes/repo-infra/archive/v0.2.0.tar.gz",
            ],
        )

    http_archive(
        name = "io_bazel_rules_docker",
        sha256 = "4521794f0fba2e20f3bf15846ab5e01d5332e587e9ce81629c7f96c793bb7036",
        strip_prefix = "rules_docker-0.14.4",
        urls = ["https://github.com/bazelbuild/rules_docker/releases/download/v0.14.4/rules_docker-v0.14.4.tar.gz"],
    )

    http_archive(
        name = "io_bazel_rules_k8s",
        strip_prefix = "rules_k8s-0.6",
        urls = ["https://github.com/bazelbuild/rules_k8s/archive/v0.6.tar.gz"],
        sha256 = "51f0977294699cd547e139ceff2396c32588575588678d2054da167691a227ef",
    )

    # https://github.com/bazelbuild/rules_nodejs
    http_archive(
        name = "build_bazel_rules_nodejs",
        sha256 = "dd4dc46066e2ce034cba0c81aa3e862b27e8e8d95871f567359f7a534cccb666",
        urls = ["https://github.com/bazelbuild/rules_nodejs/releases/download/3.1.0/rules_nodejs-3.1.0.tar.gz"],
    )

    # Python setup
    # pip_import() calls must live in WORKSPACE, otherwise we get a load() after non-load() error
    git_repository(
        name = "rules_python",
        commit = "94677401bc56ed5d756f50b441a6a5c7f735a6d4",
        remote = "https://github.com/bazelbuild/rules_python.git",
        shallow_since = "1573842889 -0500",
    )

    # TODO(fejta): get this to work
    git_repository(
        name = "io_bazel_rules_appengine",
        commit = "fdbce051adecbb369b15260046f4f23684369efc",
        remote = "https://github.com/bazelbuild/rules_appengine.git",
        shallow_since = "1552415147 -0400",
        #tag = "0.0.8+but-this-isn't-new-enough", # Latest at https://github.com/bazelbuild/rules_appengine/releases.
    )

    new_git_repository(
        name = "com_github_prometheus_operator",
        build_file_content = """
exports_files([
    "example/prometheus-operator-crd/monitoring.coreos.com_alertmanagerconfigs.yaml",
    "example/prometheus-operator-crd/monitoring.coreos.com_alertmanagers.yaml",
    "example/prometheus-operator-crd/monitoring.coreos.com_podmonitors.yaml",
    "example/prometheus-operator-crd/monitoring.coreos.com_probes.yaml",
    "example/prometheus-operator-crd/monitoring.coreos.com_prometheuses.yaml",
    "example/prometheus-operator-crd/monitoring.coreos.com_prometheusrules.yaml",
    "example/prometheus-operator-crd/monitoring.coreos.com_servicemonitors.yaml",
    "example/prometheus-operator-crd/monitoring.coreos.com_thanosrulers.yaml",
])
""",
        commit = "5555f492df250168657b72bb8cb60bec071de71f",  # Latest of release-0.45 branch
        remote = "https://github.com/prometheus-operator/prometheus-operator.git",
        shallow_since = "1610438400 +0200",
    )

    http_archive(
        name = "io_bazel_rules_jsonnet",
        sha256 = "68b5bcb0779599065da1056fc8df60d970cffe8e6832caf13819bb4d6e832459",
        strip_prefix = "rules_jsonnet-0.2.0",
        urls = ["https://github.com/bazelbuild/rules_jsonnet/archive/0.2.0.tar.gz"],
    )
