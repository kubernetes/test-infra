load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["serve_hostname.go"],
    importpath = "k8s.io/contrib/for-demos/serve_hostname",
    visibility = ["//visibility:private"],
)

go_binary(
    name = "serve_hostname",
    embed = [":go_default_library"],
    importpath = "k8s.io/contrib/for-demos/serve_hostname",
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
