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

image = "gcr.io/k8s-staging-test-infra/kubekins-e2e:v20251209-13d7d11b0f-master"

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
    "ubuntu2510": drop_unsupported_versions(kops_versions, ['1.32', '1.33']),
    "ubuntu2510arm64": drop_unsupported_versions(kops_versions, ['1.32', '1.33']),
    "amazonlinux2": kops_versions,
    "al2023": kops_versions,
    "al2023arm64": kops_versions,
    "rhel9": kops_versions,
    "rhel10arm64": drop_unsupported_versions(kops_versions, ['1.32', '1.33']),
    "rocky9": kops_versions,
    "rocky10arm64": drop_unsupported_versions(kops_versions, ['1.32', '1.33']),
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
    "rhel10": drop_unsupported_versions(kops_versions, ['1.32', '1.33']),
    "rocky10arm64": drop_unsupported_versions(kops_versions, ['1.32', '1.33']),
    "rocky10": drop_unsupported_versions(kops_versions, ['1.32', '1.33']),
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
kops29 = "v1.29.2"
kops30 = "v1.30.3"
kops31 = "v1.31.0"
kops32 = "v1.32.0"
kops33 = "v1.33.0"
kops34 = "v1.34.0"

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
