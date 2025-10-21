# Copyright 2025 The Kubernetes Authors.
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

# This file contains common variables and lists for generating kops prow jobs.

# pylint: disable=line-too-long
skip_jobs = []

image = "gcr.io/k8s-staging-test-infra/kubekins-e2e:v20251021-e2c2c9806f-master"

# Grid Definitions
networking_options = [
    "kubenet",
    "calico",
    "cilium",
    "cilium-etcd",
    "cilium-eni",
    "kopeio",
]

# GCE distributions
gce_distro_options = [
    "deb12",
    "deb12arm64",
    "deb13",
    "deb13arm64",
    "u2204",
    "u2404",
    "u2404arm64",
    "umini2404",
    "umini2404arm64",
]

# AWS distributions
distro_options = [
    "al2023",
    "deb12",
    "deb13",
    "flatcar",
    "rhel8",
    "u2204",
    "u2404",
]

k8s_versions = [
    "1.31",
    "1.32",
    "1.33",
    "1.34",
]

# kOps versions tested
kops_versions = [
    None,  # maps to latest
    "1.30",
    "1.31",
    "1.32",
]

# Distros for periodic and presubmit distro tests
distros = [
    "debian11",
    "debian12",
    "debian13",
    "ubuntu2204",
    "ubuntu2204arm64",
    "ubuntu2404",
    "ubuntu2404arm64",
    "ubuntu2510",
    "ubuntu2510arm64",
    "amazonlinux2",
    "al2023",
    "al2023arm64",
    "rhel8",
    "rhel9",
    "rocky9",
    "flatcar",
]

# Network plugins for periodic network plugin tests
network_plugins_periodics = {
    "plugins": [
        "amazon-vpc",
        "calico",
        "cilium",
        "cilium-etcd",
        "cilium-eni",
        "flannel",
        "kindnet",
        "kopeio",
        "kuberouter",
    ],
    "supports_gce": {"calico", "cilium", "kindnet"},
    "supports_azure": {"cilium"},
}

# Network plugins for presubmit network plugin tests
network_plugins_presubmits = {
    "plugins": {
        "amazonvpc": r"^(upup\/models\/cloudup\/resources\/addons\/networking\.amazon-vpc-routed-eni\/|pkg\/model\/(firewall|components\/containerd|components\/kubeproxy|iam\/iam_builder)\.go|nodeup\/pkg\/model\/kubelet\.go)",
        "calico": r"^(upup\/models\/cloudup\/resources\/addons\/networking\.projectcalico\.org\/|pkg\/model\/(components\/containerd|firewall|pki|iam\/iam_builder)\.go|nodeup\/pkg\/model\/networking\/calico\.go)",
        "cilium": r"^(upup\/models\/cloudup\/resources\/addons\/networking\.cilium\.io\/|pkg\/model\/(components\/containerd|firewall|components\/cilium|iam\/iam_builder)\.go|nodeup\/pkg\/model\/(context|networking\/cilium)\.go)",
        "cilium-etcd": r"^(upup\/models\/cloudup\/resources\/addons\/networking\.cilium\.io\/|pkg\/model\/(components\/containerd|firewall|components\/cilium|iam\/iam_builder)\.go|nodeup\/pkg\/model\/(context|networking\/cilium)\.go)",
        "cilium-eni": r"^(upup\/models\/cloudup\/resources\/addons\/networking\.cilium\.io\/|pkg\/model\/(components\/containerd|firewall|components\/cilium|iam\/iam_builder)\.go|nodeup\/pkg\/model\/(context|networking\/cilium)\.go)",
        "flannel": r"^(upup\/models\/cloudup\/resources\/addons\/networking\.flannel\/)",
        "kuberouter": r"^(upup\/models\/cloudup\/resources\/addons\/networking\.kuberouter\/|pkg\/model\/components\/containerd\.go)",
        "kindnet": r"^(upup\/models\/cloudup\/resources\/addons\/networking\.kindnet)",
    },
    "supports_ipv6": {"amazonvpc", "calico", "cilium", "kindnet"},
    "supports_gce": {"calico", "cilium", "kindnet"},
    "supports_azure": {"cilium"},
}


# Upgrade test versions
kops29 = "v1.29.2"
kops30 = "v1.30.3"
kops31 = "v1.31.0"

upgrade_versions_list = [
    #  kops    k8s          kops      k8s
    # 1.29 release branch
    ((kops29, "v1.29.8"), ("1.29", "v1.29.9")),
    # 1.30 release branch
    ((kops29, "v1.29.9"), ("1.30", "v1.30.5")),
    ((kops30, "v1.30.4"), ("1.30", "v1.30.5")),
    # kOps 1.29 upgrade to latest
    ((kops29, "v1.26.0"), ("latest", "v1.27.0")),
    ((kops29, "v1.27.0"), ("latest", "v1.28.0")),
    ((kops29, "v1.28.0"), ("latest", "v1.29.0")),
    ((kops29, "v1.29.0"), ("latest", "v1.30.0")),
    # kOps 1.30 upgrade to latest
    ((kops30, "v1.26.0"), ("latest", "v1.27.0")),
    ((kops30, "v1.27.0"), ("latest", "v1.28.0")),
    ((kops30, "v1.28.0"), ("latest", "v1.29.0")),
    ((kops30, "v1.29.0"), ("latest", "v1.30.0")),
    # kOps 1.31 upgrade to latest
    ((kops31, "v1.28.0"), ("latest", "v1.29.0")),
    ((kops31, "v1.29.0"), ("latest", "v1.30.0")),
    ((kops31, "v1.30.0"), ("latest", "v1.31.0")),
    ((kops31, "v1.31.0"), ("latest", "v1.32.0")),
    # we should have an upgrade test for every supported K8s version
    (("latest", "v1.32.0"), ("latest", "latest")),
    (("latest", "v1.31.0"), ("latest", "v1.32.0")),
    (("latest", "v1.30.0"), ("latest", "v1.31.0")),
    (("latest", "v1.29.0"), ("latest", "v1.30.0")),
    (("latest", "v1.28.0"), ("latest", "v1.29.0")),
    (("latest", "v1.27.0"), ("latest", "v1.28.0")),
    (("latest", "v1.26.0"), ("latest", "v1.27.0")),
    # kOps latest should always be able to upgrade from stable to latest and stable to ci
    (("latest", "stable"), ("latest", "latest")),
    (("latest", "stable"), ("latest", "ci")),
]
