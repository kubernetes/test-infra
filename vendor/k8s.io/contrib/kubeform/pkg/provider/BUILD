load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["provider.go"],
    importpath = "k8s.io/contrib/kubeform/pkg/provider",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/hashicorp/terraform/helper/schema:go_default_library",
        "//vendor/github.com/hashicorp/terraform/terraform:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/api:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/client/clientset_generated/release_1_4:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/client/restclient:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/client/unversioned/clientcmd:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/client/unversioned/clientcmd/api:go_default_library",
    ],
)

filegroup(
    name = "package-srcs",
    srcs = glob(["**"]),
    tags = ["automanaged"],
    visibility = ["//visibility:private"],
)

filegroup(
    name = "all-srcs",
    srcs = [":package-srcs"],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
