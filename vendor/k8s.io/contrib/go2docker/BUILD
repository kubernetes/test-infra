load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "cacerts.go",
        "go2docker.go",
    ],
    importpath = "k8s.io/contrib/go2docker",
    visibility = ["//visibility:private"],
)

go_binary(
    name = "go2docker",
    embed = [":go_default_library"],
    importpath = "k8s.io/contrib/go2docker",
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
