load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "api.go",
        "customizations.go",
        "doc.go",
        "doc_custom.go",
        "errors.go",
        "service.go",
        "waiters.go",
    ],
    importpath = "github.com/aws/aws-sdk-go/service/dynamodb",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/aws/aws-sdk-go/aws:go_default_library",
        "//vendor/github.com/aws/aws-sdk-go/aws/awserr:go_default_library",
        "//vendor/github.com/aws/aws-sdk-go/aws/awsutil:go_default_library",
        "//vendor/github.com/aws/aws-sdk-go/aws/client:go_default_library",
        "//vendor/github.com/aws/aws-sdk-go/aws/client/metadata:go_default_library",
        "//vendor/github.com/aws/aws-sdk-go/aws/request:go_default_library",
        "//vendor/github.com/aws/aws-sdk-go/aws/signer/v4:go_default_library",
        "//vendor/github.com/aws/aws-sdk-go/private/protocol:go_default_library",
        "//vendor/github.com/aws/aws-sdk-go/private/protocol/jsonrpc:go_default_library",
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
        "//vendor/github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute:all-srcs",
        "//vendor/github.com/aws/aws-sdk-go/service/dynamodb/dynamodbiface:all-srcs",
        "//vendor/github.com/aws/aws-sdk-go/service/dynamodb/expression:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
