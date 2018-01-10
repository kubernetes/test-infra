load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["stream.go"],
    importpath = "github.com/beorn7/perks/quantile",
    tags = ["manual"],
    visibility = ["//visibility:public"],
)

filegroup(
    name = "package-srcs",
    srcs = glob(["**"]),
    tags = [
        "automanaged",
        "manual",
    ],
    visibility = ["//visibility:private"],
)

filegroup(
    name = "all-srcs",
    srcs = [":package-srcs"],
    tags = [
        "automanaged",
        "manual",
    ],
    visibility = ["//visibility:public"],
)
