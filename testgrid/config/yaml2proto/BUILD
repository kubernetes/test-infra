package(default_visibility = ["//visibility:public"])

licenses(["notice"])

load(
    "@io_bazel_rules_go//go:def.bzl",
    "go_library",
    "go_test",
)

go_test(
    name = "go_default_test",
    srcs = ["yaml2proto_test.go"],
    library = ":go_default_library",
    tags = ["automanaged"],
    deps = [
        "//testgrid/config/pb:go_default_library",
        "//vendor:github.com/golang/protobuf/proto",
    ],
)

go_library(
    name = "go_default_library",
    srcs = ["yaml2proto.go"],
    tags = ["automanaged"],
    deps = [
        "//testgrid/config/pb:go_default_library",
        "//vendor:github.com/golang/protobuf/proto",
        "//vendor:gopkg.in/yaml.v2",
    ],
)
