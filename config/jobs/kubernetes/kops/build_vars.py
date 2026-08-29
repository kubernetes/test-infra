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

image = "us-central1-docker.pkg.dev/k8s-staging-test-infra/images/kubekins-e2e:v20260824-a5e45c393a-master"

k8s_versions = [
    "master",
    "1.35",
    "1.36",
    "1.37",
]

# kOps versions tested
kops_versions = [
    None,  # maps to latest
    "1.35",
    "1.36",
    "1.37",
]

def drop_unsupported_versions(original_list, version_to_drop):
    return list(filter(lambda item: item not in version_to_drop, original_list))

# AWS distributions
aws_distro_options = {
    # containerd 2.x needs GLIBC_2.32; Debian 11 is on 2.31, so it cannot run any
    # containerd that kops 1.37 still accepts (2.1.0 is the floor). Spell out the
    # kops versions that can still run it instead of dropping the None entry, so
    # that aging those versions out takes the special case with them rather than
    # silently handing deb11 every new kops release.
    "debian11": ['1.35', '1.36'],
    "debian12": kops_versions,
    "debian13": kops_versions,
    "ubuntu2204": kops_versions,
    "ubuntu2204arm64": kops_versions,
    "ubuntu2404": kops_versions,
    "ubuntu2404arm64": kops_versions,
    "ubuntu2510": kops_versions,
    "ubuntu2510arm64": kops_versions,
    "ubuntu2604": kops_versions,
    "ubuntu2604arm64": kops_versions,
    "al2023": kops_versions,
    "al2023arm64": kops_versions,
    "rhel9": kops_versions,
    "rhel10arm64": kops_versions,
    "rocky9": kops_versions,
    "rocky10arm64": kops_versions,
    "flatcar": kops_versions,
}

# GCE distributions
gce_distro_options = {
    # COS 121 was pinned down to containerd 2.0.7 and cannot reach the 2.1.0 floor
    # kops 1.37 requires. Allowlisted as for debian11 above.
    "cos121": ['1.35'],
    "cos121arm64": ['1.35'],
    "cos125": kops_versions,
    "cos125arm64": kops_versions,
    "cos129": drop_unsupported_versions(kops_versions, ['1.35']),
    "cos129arm64": drop_unsupported_versions(kops_versions, ['1.35']),
    "cosdev": kops_versions,
    "cosdevarm64": kops_versions,
    "debian12": kops_versions,
    "debian12arm64": kops_versions,
    "debian13": kops_versions,
    "debian13arm64": kops_versions,
    "ubuntu2204": kops_versions,
    "ubuntu2404": kops_versions,
    "ubuntu2404arm64": kops_versions,
    "ubuntuminimal2404": kops_versions,
    "ubuntuminimal2404arm64": kops_versions,
    "rhel10": kops_versions,
    "rocky10arm64": kops_versions,
    "rocky10": kops_versions,
}


# Network plugins for periodic network plugin tests
network_plugins_periodics = {
    "plugins": [
        "amazon-vpc",
        "calico",
        "gce",
        "cilium",
        "cilium-etcd",
        "cilium-eni",
        "flannel",
        "kindnet",
        "kubenet",
        "kuberouter",
    ],
    "supports_aws": [
        "amazon-vpc",
        "calico",
        "cilium",
        "cilium-etcd",
        "cilium-eni",
        "flannel",
        "kindnet",
        "kubenet",
        "kuberouter",
    ],
    "supports_gce": ["kubenet", "calico", "cilium", "cilium-etcd", "kindnet", "gce"],
    "supports_azure": ["kubenet", "calico", "cilium", "kindnet"],
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
        "kubenet": r"^pkg\/apis\/kops\/networking.go",
    },
    "supports_ipv6": {"amazonvpc", "calico", "cilium", "kindnet"},
    "supports_gce": {"calico", "cilium", "cilium-etcd", "kindnet"},
    "supports_azure": {"calico", "cilium", "kubenet", "kindnet"},
    "supports_aws": {"amazonvpc", "calico", "cilium", "cilium-etcd", "flannel", "cilium-eni", "kindnet", "kubenet", "kuberouter"}
}


# Upgrade test versions
kops32 = "v1.32.4"
kops33 = "v1.33.2"
kops35 = "v1.35.2"
kops36 = "v1.36.2"

upgrade_versions_list = [
    #  kops    k8s          kops      k8s
    # 1.35 release branch
    ((kops35, "v1.35.6"), ("1.35", "v1.35.7")),
    # 1.36 release branch
    ((kops35, "v1.35.7"), ("1.36", "v1.36.3")),
    ((kops36, "v1.36.3"), ("1.36", "v1.36.3")),
    # 1.37 release branch
    ((kops36, "v1.36.4"), ("1.37", "v1.37.0")),
    # kOps 1.35 upgrade to latest
    ((kops35, "v1.31.14"), ("latest", "v1.32.13")),
    ((kops35, "v1.32.13"), ("latest", "v1.32.13")),
    ((kops35, "v1.33.13"), ("latest", "v1.34.10")),
    ((kops35, "v1.34.10"), ("latest", "v1.35.7")),
    # kOps 1.36 upgrade to latest
    ((kops36, "v1.32.13"), ("latest", "v1.33.13")),
    ((kops36, "v1.33.13"), ("latest", "v1.34.10")),
    ((kops36, "v1.34.10"), ("latest", "v1.35.7")),
    ((kops36, "v1.35.7"), ("latest", "v1.36.3")),
    # we should have an upgrade test for every supported K8s version
    (("latest", "v1.37.0"), ("latest", "latest")),
    (("latest", "v1.36.0"), ("latest", "v1.37.0")),
    (("latest", "v1.35.0"), ("latest", "v1.36.0")),
    (("latest", "v1.34.0"), ("latest", "v1.35.0")),
    (("latest", "v1.33.0"), ("latest", "v1.34.0")),
    # kOps latest should always be able to upgrade from stable to latest and stable to ci
    (("latest", "stable"), ("latest", "latest")),
    (("latest", "stable"), ("latest", "ci")),
]
