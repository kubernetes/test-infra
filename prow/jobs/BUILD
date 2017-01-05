package(default_visibility = ["//visibility:public"])

licenses(["notice"])

load(
    "@io_bazel_rules_go//go:def.bzl",
    "go_library",
    "go_test",
)

go_test(
    name = "go_default_test",
    srcs = ["jobs_test.go"],
    data = [
        "//jobs",
        "//prow:configs",
        "//scenarios",
    ],
    library = ":go_default_library",
    tags = ["automanaged"],
)

go_library(
    name = "go_default_library",
    srcs = ["jobs.go"],
    tags = ["automanaged"],
    deps = [
        "//prow/kube:go_default_library",
        "//vendor:github.com/Sirupsen/logrus",
        "//vendor:github.com/ghodss/yaml",
    ],
)
