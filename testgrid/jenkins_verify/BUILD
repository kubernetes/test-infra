package(default_visibility = ["//visibility:public"])

licenses(["notice"])

load(
    "@io_bazel_rules_go//go:def.bzl",
    "go_binary",
)

go_binary(
    name = "jenkins_verify",
    srcs = ["jenkins_validate.go"],
    tags = ["automanaged"],
    deps = [
        "//testgrid/config/pb:go_default_library",
        "//testgrid/config/yaml2proto:go_default_library",
        "//vendor:github.com/golang/protobuf/proto",
    ],
)
