package(default_visibility = ["//visibility:public"])

load(
    "@io_bazel_rules_go//go:def.bzl",
    "go_binary",
    "go_library",
    "go_test",
)

go_binary(
    name = "boskos",
    embed = [":go_default_library"],
    importpath = "k8s.io/test-infra/boskos",
)

go_test(
    name = "go_default_test",
    srcs = ["boskos_test.go"],
    data = ["resources.json"],
    embed = [":go_default_library"],
    importpath = "k8s.io/test-infra/boskos",
    deps = [
        "//boskos/common:go_default_library",
        "//boskos/ranch:go_default_library",
    ],
)

go_library(
    name = "go_default_library",
    srcs = ["boskos.go"],
    importpath = "k8s.io/test-infra/boskos",
    deps = [
        "//boskos/ranch:go_default_library",
        "//vendor/github.com/sirupsen/logrus:go_default_library",
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
        "//boskos/client:all-srcs",
        "//boskos/common:all-srcs",
        "//boskos/janitor:all-srcs",
        "//boskos/metrics:all-srcs",
        "//boskos/ranch:all-srcs",
        "//boskos/reaper:all-srcs",
    ],
    tags = ["automanaged"],
)
