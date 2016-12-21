package(default_visibility = ["//visibility:public"])

licenses(["notice"])

load(
    "@io_bazel_rules_go//go:def.bzl",
    "go_library",
    "go_test",
)

go_test(
    name = "go_default_test",
    srcs = [
        "plugins_test.go",
        "respond_test.go",
    ],
    library = ":go_default_library",
    tags = ["automanaged"],
    deps = ["//prow/github:go_default_library"],
)

go_library(
    name = "go_default_library",
    srcs = [
        "plugins.go",
        "respond.go",
    ],
    tags = ["automanaged"],
    deps = [
        "//prow/github:go_default_library",
        "//prow/jobs:go_default_library",
        "//prow/kube:go_default_library",
        "//vendor:github.com/Sirupsen/logrus",
        "//vendor:github.com/ghodss/yaml",
    ],
)
