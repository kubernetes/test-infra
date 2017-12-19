load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["contrib.go"],
    importpath = "golang.org/x/tools/cmd/go-contrib-init",
    visibility = ["//visibility:private"],
)

go_binary(
    name = "go-contrib-init",
    embed = [":go_default_library"],
    importpath = "golang.org/x/tools/cmd/go-contrib-init",
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
