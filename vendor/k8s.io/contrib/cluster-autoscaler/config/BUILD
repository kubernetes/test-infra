load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "kubernetes.go",
        "migconfig.go",
    ],
    importpath = "k8s.io/contrib/cluster-autoscaler/config",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/k8s.io/contrib/cluster-autoscaler/utils/gce_url:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/api:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/api/unversioned:go_default_library",
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
