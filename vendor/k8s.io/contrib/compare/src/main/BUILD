load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["result-comparator.go"],
    importpath = "k8s.io/contrib/compare/src/main",
    visibility = ["//visibility:private"],
    deps = [
        "//vendor/github.com/daviddengcn/go-colortext:go_default_library",
        "//vendor/github.com/spf13/pflag:go_default_library",
        "//vendor/k8s.io/contrib/compare/src:go_default_library",
        "//vendor/k8s.io/contrib/test-utils/utils:go_default_library",
    ],
)

go_binary(
    name = "main",
    embed = [":go_default_library"],
    importpath = "k8s.io/contrib/compare/src/main",
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
