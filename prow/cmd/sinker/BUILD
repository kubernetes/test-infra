package(default_visibility = ["//visibility:public"])

licenses(["notice"])

load(
    "@io_bazel_rules_go//go:def.bzl",
    "go_binary",
    "go_test",
)

go_binary(
    name = "sinker",
    srcs = ["main.go"],
    tags = ["automanaged"],
    deps = [
        "//prow/kube:go_default_library",
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
        "//prow/kube:go_default_library",
        "//vendor:github.com/Sirupsen/logrus",
    ],
)
