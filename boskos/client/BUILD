package(default_visibility = ["//visibility:public"])

load(
    "@io_bazel_rules_go//go:def.bzl",
    "go_library",
    "go_test",
)

go_test(
    name = "go_default_test",
    srcs = ["client_test.go"],
    importpath = "k8s.io/test-infra/boskos/client",
    library = ":go_default_library",
    deps = ["//boskos/common:go_default_library"],
)

go_library(
    name = "go_default_library",
    srcs = ["client.go"],
    importpath = "k8s.io/test-infra/boskos/client",
    deps = ["//boskos/common:go_default_library"],
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
