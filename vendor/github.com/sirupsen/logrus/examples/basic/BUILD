load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["basic.go"],
    importpath = "github.com/sirupsen/logrus/examples/basic",
    visibility = ["//visibility:private"],
    deps = ["//vendor/github.com/Sirupsen/logrus:go_default_library"],
)

go_binary(
    name = "basic",
    embed = [":go_default_library"],
    importpath = "github.com/sirupsen/logrus/examples/basic",
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
