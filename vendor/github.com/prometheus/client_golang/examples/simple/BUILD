load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["main.go"],
    importpath = "github.com/prometheus/client_golang/examples/simple",
    visibility = ["//visibility:private"],
    deps = ["//vendor/github.com/prometheus/client_golang/prometheus:go_default_library"],
)

go_binary(
    name = "simple",
    embed = [":go_default_library"],
    importpath = "github.com/prometheus/client_golang/examples/simple",
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
