package(default_visibility = ["//visibility:public"])

load(
    "@io_bazel_rules_go//go:def.bzl",
    "go_library",
    "go_test",
)

go_test(
    name = "go_default_test",
    srcs = ["yaml2proto_test.go"],
    library = ":go_default_library",
)

go_library(
    name = "go_default_library",
    srcs = ["yaml2proto.go"],
    deps = [
        "//testgrid/config/pb:go_default_library",
        "//vendor/github.com/golang/protobuf/proto:go_default_library",
        "//vendor/gopkg.in/yaml.v2:go_default_library",
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
)
