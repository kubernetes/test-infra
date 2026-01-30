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

image = "us-central1-docker.pkg.dev/k8s-staging-test-infra/images/kubekins-e2e:v20260127-c1affcc8de-master"

k8s_versions = [
    "master",
    "1.32",
    "1.33",
    "1.34",
    "1.35",
]

# kOps versions tested
kops_versions = [
    None,  # maps to latest
    "1.32",
    "1.33",
    "1.34",
]

def drop_unsupported_versions(original_list, version_to_drop):
    return list(filter(lambda item: item not in version_to_drop, original_list))

# AWS distributions
aws_distro_options = {
    "debian11": kops_versions,
    "debian12": kops_versions,
    "debian13": kops_versions,
    "ubuntu2204": kops_versions,
    "ubuntu2204arm64": kops_versions,
    "ubuntu2404": kops_versions,
    "ubuntu2404arm64": kops_versions,
    "ubuntu2510": drop_unsupported_versions(kops_versions, ['1.32', '1.33', '1.34']),
    "ubuntu2510arm64": drop_unsupported_versions(kops_versions, ['1.32', '1.33', '1.34']),
    "al2023": kops_versions,
    "al2023arm64": kops_versions,
    "rhel9": kops_versions,
    "rhel10arm64": drop_unsupported_versions(kops_versions, ['1.32', '1.33', '1.34']),
    "rocky9": kops_versions,
    "rocky10arm64": drop_unsupported_versions(kops_versions, ['1.32', '1.33', '1.34']),
    "flatcar": kops_versions,
}

# GCE distributions
gce_distro_options = {
    "cos121": kops_versions,
    "cos121arm64": drop_unsupported_versions(kops_versions, ['1.32']),
    "cos125": kops_versions,
    "cos125arm64": drop_unsupported_versions(kops_versions, ['1.32']),
    "cosdev": kops_versions,
    "cosdevarm64": drop_unsupported_versions(kops_versions, ['1.32']),
    "debian12": kops_versions,
    "debian12arm64": drop_unsupported_versions(kops_versions, ['1.32']),
    "debian13": kops_versions,
    "debian13arm64": drop_unsupported_versions(kops_versions, ['1.32']),
    "ubuntu2204": kops_versions,
    "ubuntu2404": kops_versions,
    "ubuntu2404arm64": drop_unsupported_versions(kops_versions, ['1.32']),
    "ubuntuminimal2404": kops_versions,
    "ubuntuminimal2404arm64": drop_unsupported_versions(kops_versions, ['1.32']),
    "rhel10": drop_unsupported_versions(kops_versions, ['1.32', '1.33', '1.34']),
    "rocky10arm64": drop_unsupported_versions(kops_versions, ['1.32', '1.33', '1.34']),
    "rocky10": drop_unsupported_versions(kops_versions, ['1.32', '1.33', '1.34']),
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
        "kopeio",
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
        "kopeio",
        "kubenet",
        "kuberouter",
    ],
    "supports_gce": ["kubenet", "calico", "cilium", "kindnet", "gce"],
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
    "supports_gce": {"calico", "cilium", "kindnet"},
    "supports_azure": {"calico", "cilium", "kubenet", "kindnet"},
    "supports_aws": {"amazonvpc", "calico", "cilium", "cilium-etcd", "flannel", "cilium-eni", "kindnet", "kubenet", "kuberouter"}
}


# Upgrade test versions
kops30 = "v1.30.4"
kops31 = "v1.31.0"
kops32 = "v1.32.2"
kops33 = "v1.33.1"
kops34 = "v1.34.0"

upgrade_versions_list = [
    #  kops    k8s          kops      k8s
    # 1.33 release branch
    ((kops33, "v1.33.6"), ("1.33", "v1.33.7")),
    # 1.34 release branch
    ((kops33, "v1.33.7"), ("1.34", "v1.34.3")),
    ((kops34, "v1.34.2"), ("1.34", "v1.34.3")),
    # kOps 1.32 upgrade to latest
    ((kops32, "v1.28.15"), ("latest", "v1.29.15")),
    ((kops32, "v1.29.15"), ("latest", "v1.30.14")),
    ((kops32, "v1.30.14"), ("latest", "v1.31.14")),
    ((kops32, "v1.31.14"), ("latest", "v1.32.11")),
    # kOps 1.33 upgrade to latest
    ((kops33, "v1.29.15"), ("latest", "v1.30.14")),
    ((kops33, "v1.30.14"), ("latest", "v1.31.14")),
    ((kops33, "v1.31.14"), ("latest", "v1.32.11")),
    ((kops33, "v1.32.11"), ("latest", "v1.33.7")),
    # kOps 1.34 upgrade to latest
    ((kops34, "v1.30.14"), ("latest", "v1.31.14")),
    ((kops34, "v1.31.14"), ("latest", "v1.32.11")),
    ((kops34, "v1.32.11"), ("latest", "v1.33.7")),
    ((kops34, "v1.33.7"), ("latest", "v1.34.3")),
    # we should have an upgrade test for every supported K8s version
    (("latest", "v1.34.0"), ("latest", "latest")),
    (("latest", "v1.33.0"), ("latest", "v1.34.0")),
    (("latest", "v1.32.0"), ("latest", "v1.33.0")),
    (("latest", "v1.31.0"), ("latest", "v1.32.0")),
    (("latest", "v1.30.0"), ("latest", "v1.31.0")),
    (("latest", "v1.29.0"), ("latest", "v1.30.0")),
    # kOps latest should always be able to upgrade from stable to latest and stable to ci
    (("latest", "stable"), ("latest", "latest")),
    (("latest", "stable"), ("latest", "ci")),
]
