load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["queryutil.go"],
    importpath = "github.com/aws/aws-sdk-go/private/protocol/query/queryutil",
    tags = ["manual"],
    visibility = ["//visibility:public"],
    deps = ["//vendor/github.com/aws/aws-sdk-go/private/protocol:go_default_library"],
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
