package(default_visibility = ["//visibility:public"])

licenses(["notice"])

load(
    "@io_bazel_rules_go//go:def.bzl",
    "go_binary",
    "go_test",
)

go_binary(
    name = "hook",
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

go_test(
    name = "go_default_test",
    srcs = [
        "events.go",
        "main.go",
        "main_test.go",
        "server.go",
        "server_test.go",
    ],
    data = [
        "//prow:configs",
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
