load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["main.go"],
    importpath = "k8s.io/test-infra/experiment/needs-rebase",
    visibility = ["//visibility:private"],
    deps = [
        "//experiment/needs-rebase/plugin:go_default_library",
        "//prow/github:go_default_library",
        "//prow/hook:go_default_library",
        "//prow/pluginhelp/externalplugins:go_default_library",
        "//prow/plugins:go_default_library",
        "//vendor/github.com/sirupsen/logrus:go_default_library",
    ],
)

go_binary(
    name = "needs-rebase",
    embed = [":go_default_library"],
    importpath = "k8s.io/test-infra/experiment/needs-rebase",
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
        "//experiment/needs-rebase/plugin:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
