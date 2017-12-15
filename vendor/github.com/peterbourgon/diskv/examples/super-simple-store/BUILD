load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["super-simple-store.go"],
    importpath = "github.com/peterbourgon/diskv/examples/super-simple-store",
    visibility = ["//visibility:private"],
    deps = ["//vendor/github.com/peterbourgon/diskv:go_default_library"],
)

go_binary(
    name = "super-simple-store",
    embed = [":go_default_library"],
    importpath = "github.com/peterbourgon/diskv/examples/super-simple-store",
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
