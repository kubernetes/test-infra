package(default_visibility = ["//visibility:public"])

licenses(["notice"])

load(
    "@io_bazel_rules_go//go:def.bzl",
    "go_binary",
    "go_library",
    "go_test",
)

go_binary(
    name = "janitor",
    library = ":go_default_library",
    tags = ["automanaged"],
)

go_library(
    name = "go_default_library",
    srcs = ["janitor.go"],
    tags = ["automanaged"],
    deps = [
        "//boskos/client:go_default_library",
        "//boskos/common:go_default_library",
        "//vendor/github.com/Sirupsen/logrus:go_default_library",
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

go_test(
    name = "go_default_test",
    srcs = ["janitor_test.go"],
    library = ":go_default_library",
    tags = ["automanaged"],
    deps = ["//boskos/common:go_default_library"],
)
