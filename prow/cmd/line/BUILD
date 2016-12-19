package(default_visibility = ["//visibility:public"])

licenses(["notice"])

load(
    "@io_bazel_rules_go//go:def.bzl",
    "go_binary",
    "go_test",
)

go_binary(
    name = "line",
    srcs = ["main.go"],
    tags = ["automanaged"],
    deps = [
        "//prow/github:go_default_library",
        "//prow/jenkins:go_default_library",
        "//prow/jobs:go_default_library",
        "//prow/kube:go_default_library",
        "//prow/line:go_default_library",
        "//vendor:github.com/Sirupsen/logrus",
    ],
)

go_test(
    name = "go_default_test",
    srcs = [
        "main.go",
        "main_test.go",
    ],
    tags = ["automanaged"],
    deps = [
        "//prow/github:go_default_library",
        "//prow/github/fakegithub:go_default_library",
        "//prow/jenkins:go_default_library",
        "//prow/jobs:go_default_library",
        "//prow/kube:go_default_library",
        "//prow/line:go_default_library",
        "//vendor:github.com/Sirupsen/logrus",
    ],
)
