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
        "config_test.go",
        "jobs_test.go",
    ],
    data = [
        "//jobs",
        "//mungegithub:configmaps",
        "//prow:configs",
        "//scenarios",
    ],
    library = ":go_default_library",
    tags = ["automanaged"],
    deps = [
        "//prow/kube:go_default_library",
        "//vendor/github.com/ghodss/yaml:go_default_library",
    ],
)

go_library(
    name = "go_default_library",
    srcs = [
        "agent.go",
        "config.go",
        "jobs.go",
    ],
    tags = ["automanaged"],
    deps = [
        "//prow/kube:go_default_library",
        "//vendor/github.com/Sirupsen/logrus:go_default_library",
        "//vendor/github.com/ghodss/yaml:go_default_library",
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
