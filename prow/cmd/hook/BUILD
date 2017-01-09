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
    library = ":go_default_library",
    tags = ["automanaged"],
    data = [
        "//prow:configs",
    ],
)

go_test(
    name = "go_default_test",
    srcs = [
        "main_test.go",
        "server_test.go",
    ],
    data = [
        "//prow:configs",
    ],
    library = ":go_default_library",
    tags = ["automanaged"],
    deps = ["//prow/plugins:go_default_library"],
)

go_library(
    name = "go_default_library",
    srcs = [
        "events.go",
        "main.go",
        "server.go",
    ],
    tags = ["automanaged"],
    deps = [
        "//prow/github:go_default_library",
        "//prow/jobs:go_default_library",
        "//prow/kube:go_default_library",
        "//prow/plugins:go_default_library",
        "//prow/plugins/cla:go_default_library",
        "//prow/plugins/close:go_default_library",
        "//prow/plugins/lgtm:go_default_library",
        "//prow/plugins/releasenote:go_default_library",
        "//prow/plugins/trigger:go_default_library",
        "//vendor:github.com/Sirupsen/logrus",
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
