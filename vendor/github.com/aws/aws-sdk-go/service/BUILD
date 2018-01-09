load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["generate.go"],
    importpath = "github.com/aws/aws-sdk-go/service",
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
    srcs = [
        ":package-srcs",
        "//vendor/github.com/aws/aws-sdk-go/service/autoscaling:all-srcs",
        "//vendor/github.com/aws/aws-sdk-go/service/ec2:all-srcs",
        "//vendor/github.com/aws/aws-sdk-go/service/iam:all-srcs",
        "//vendor/github.com/aws/aws-sdk-go/service/s3:all-srcs",
        "//vendor/github.com/aws/aws-sdk-go/service/sts:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
