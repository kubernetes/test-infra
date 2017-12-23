load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["ex1.go"],
    importpath = "github.com/petar/GoLLRB/example",
    visibility = ["//visibility:private"],
    deps = ["//vendor/github.com/petar/GoLLRB/llrb:go_default_library"],
)

go_binary(
    name = "example",
    embed = [":go_default_library"],
    importpath = "github.com/petar/GoLLRB/example",
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
