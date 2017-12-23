load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["cloud_provider.go"],
    importpath = "k8s.io/contrib/cluster-autoscaler/cloudprovider",
    visibility = ["//visibility:public"],
    deps = ["//vendor/k8s.io/kubernetes/pkg/api:go_default_library"],
)

filegroup(
    name = "package-srcs",
    srcs = glob(["**"]),
    tags = ["automanaged"],
    visibility = ["//visibility:private"],
)

filegroup(
    name = "all-srcs",
    srcs = [
        ":package-srcs",
        "//vendor/k8s.io/contrib/cluster-autoscaler/cloudprovider/aws:all-srcs",
        "//vendor/k8s.io/contrib/cluster-autoscaler/cloudprovider/gce:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
