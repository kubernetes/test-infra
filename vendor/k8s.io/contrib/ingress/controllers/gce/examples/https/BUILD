load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["make_secret.go"],
    importpath = "k8s.io/contrib/ingress/controllers/gce/examples/https",
    visibility = ["//visibility:private"],
    deps = [
        "//vendor/k8s.io/kubernetes/pkg/api:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/api/install:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/apimachinery/registered:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/runtime:go_default_library",
    ],
)

go_binary(
    name = "https",
    embed = [":go_default_library"],
    importpath = "k8s.io/contrib/ingress/controllers/gce/examples/https",
    visibility = ["//visibility:public"],
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
