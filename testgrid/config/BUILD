package(default_visibility = ["//visibility:public"])

licenses(["notice"])

load(
    "@io_bazel_rules_go//go:def.bzl",
    "go_binary",
    "go_library",
    "go_test",
)

go_binary(
    name = "config",
    library = ":go_default_library",
    tags = ["automanaged"],
)

go_test(
    name = "go_default_test",
    srcs = ["config_test.go"],
    data = [
        "config.yaml",
    ],
    library = ":go_default_library",
    tags = ["automanaged"],
    deps = [
        "//testgrid/config/pb:go_default_library",
        "//testgrid/config/yaml2proto:go_default_library",
        "//vendor:github.com/golang/protobuf/proto",
    ],
)

go_library(
    name = "go_default_library",
    srcs = ["main.go"],
    tags = ["automanaged"],
    deps = ["//testgrid/config/yaml2proto:go_default_library"],
)
