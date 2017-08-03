package(default_visibility = ["//visibility:public"])

licenses(["notice"])

load(
    "@io_bazel_rules_go//go:def.bzl",
    "go_binary",
    "go_library",
    "go_test",
)

go_binary(
    name = "hook",
    data = [
        "//prow:configs",
    ],
    library = ":go_default_library",
)

go_test(
    name = "go_default_test",
    srcs = ["main_test.go"],
    data = [
        "//prow:configs",
    ],
    library = ":go_default_library",
    deps = ["//prow/plugins:go_default_library"],
)

go_library(
    name = "go_default_library",
    srcs = ["main.go"],
    deps = [
        "//prow/config:go_default_library",
        "//prow/git:go_default_library",
        "//prow/github:go_default_library",
        "//prow/hook:go_default_library",
        "//prow/kube:go_default_library",
        "//prow/plugins:go_default_library",
        "//prow/plugins/assign:go_default_library",
        "//prow/plugins/cla:go_default_library",
        "//prow/plugins/close:go_default_library",
        "//prow/plugins/golint:go_default_library",
        "//prow/plugins/heart:go_default_library",
        "//prow/plugins/label:go_default_library",
        "//prow/plugins/lgtm:go_default_library",
        "//prow/plugins/releasenote:go_default_library",
        "//prow/plugins/reopen:go_default_library",
        "//prow/plugins/trigger:go_default_library",
        "//prow/plugins/updateconfig:go_default_library",
        "//prow/plugins/yuks:go_default_library",
        "//prow/slack:go_default_library",
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
