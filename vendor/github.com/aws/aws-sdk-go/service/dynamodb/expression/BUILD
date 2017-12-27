load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "condition.go",
        "doc.go",
        "error.go",
        "expression.go",
        "key_condition.go",
        "operand.go",
        "projection.go",
        "update.go",
    ],
    importpath = "github.com/aws/aws-sdk-go/service/dynamodb/expression",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/aws/aws-sdk-go/aws:go_default_library",
        "//vendor/github.com/aws/aws-sdk-go/service/dynamodb:go_default_library",
        "//vendor/github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute:go_default_library",
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
    srcs = [":package-srcs"],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
