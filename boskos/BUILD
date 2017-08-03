package(default_visibility = ["//visibility:public"])

load(
    "@io_bazel_rules_go//go:def.bzl",
    "go_binary",
    "go_library",
    "go_test",
)

go_binary(
    name = "boskos",
    library = ":go_default_library",
)

go_test(
    name = "go_default_test",
    srcs = ["boskos_test.go"],
    data = ["resources.json"],
    library = ":go_default_library",
    deps = [
        "//boskos/common:go_default_library",
        "//boskos/ranch:go_default_library",
    ],
)

go_library(
    name = "go_default_library",
    srcs = ["boskos.go"],
    deps = [
        "//boskos/ranch:go_default_library",
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
    srcs = [
        ":package-srcs",
        "//boskos/client:all-srcs",
        "//boskos/common:all-srcs",
        "//boskos/janitor:all-srcs",
        "//boskos/ranch:all-srcs",
        "//boskos/reaper:all-srcs",
    ],
    tags = ["automanaged"],
)
