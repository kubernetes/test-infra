package(default_visibility = ["//visibility:public"])

licenses(["notice"])

load(
    "@io_bazel_rules_go//go:def.bzl",
    "go_binary",
    "go_test",
)

go_binary(
    name = "config",
    srcs = ["main.go"],
    tags = ["automanaged"],
    deps = ["//testgrid/config/yaml2proto:go_default_library"],
)

go_test(
    name = "go_default_test",
    srcs = [
        "config_test.go",
        "main.go",
    ],
    data = [
        "config.yaml",
    ],
    tags = ["automanaged"],
    deps = [
        "//testgrid/config/pb:go_default_library",
        "//testgrid/config/yaml2proto:go_default_library",
        "//vendor:github.com/golang/protobuf/proto",
    ],
)
