load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "link_grpc.go",
        "main.go",
    ],
    importpath = "github.com/golang/protobuf/protoc-gen-go",
    visibility = ["//visibility:private"],
    deps = [
        "//vendor/github.com/golang/protobuf/proto:go_default_library",
        "//vendor/github.com/golang/protobuf/protoc-gen-go/generator:go_default_library",
        "//vendor/github.com/golang/protobuf/protoc-gen-go/grpc:go_default_library",
    ],
)

go_binary(
    name = "protoc-gen-go",
    embed = [":go_default_library"],
    importpath = "github.com/golang/protobuf/protoc-gen-go",
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
        "//vendor/github.com/golang/protobuf/protoc-gen-go/descriptor:all-srcs",
        "//vendor/github.com/golang/protobuf/protoc-gen-go/generator:all-srcs",
        "//vendor/github.com/golang/protobuf/protoc-gen-go/grpc:all-srcs",
        "//vendor/github.com/golang/protobuf/protoc-gen-go/plugin:all-srcs",
        "//vendor/github.com/golang/protobuf/protoc-gen-go/testdata/my_test:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
