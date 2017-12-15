load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["orange.go"],
    importpath = "golang.org/x/tools/cmd/fiximports/testdata/src/fruit.io/orange",
    visibility = ["//visibility:public"],
    deps = ["//vendor/fruit.io/pear:go_default_library"],
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
