load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["netperf_tester.go"],
    importpath = "k8s.io/contrib/netperf-tester",
    visibility = ["//visibility:private"],
)

go_binary(
    name = "netperf-tester",
    embed = [":go_default_library"],
    importpath = "k8s.io/contrib/netperf-tester",
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
