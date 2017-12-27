load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "assert.go",
        "client.go",
        "custom_ca_bundle.go",
        "util.go",
    ],
    importpath = "github.com/aws/aws-sdk-go/awstesting",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/aws/aws-sdk-go/aws:go_default_library",
        "//vendor/github.com/aws/aws-sdk-go/aws/client:go_default_library",
        "//vendor/github.com/aws/aws-sdk-go/aws/client/metadata:go_default_library",
        "//vendor/github.com/aws/aws-sdk-go/aws/defaults:go_default_library",
        "//vendor/github.com/aws/aws-sdk-go/private/util:go_default_library",
    ],
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
        "//vendor/github.com/aws/aws-sdk-go/awstesting/integration/customizations/s3:all-srcs",
        "//vendor/github.com/aws/aws-sdk-go/awstesting/mock:all-srcs",
        "//vendor/github.com/aws/aws-sdk-go/awstesting/unit:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
