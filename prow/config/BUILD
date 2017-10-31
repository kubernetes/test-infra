package(default_visibility = ["//visibility:public"])

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
    importpath = "k8s.io/test-infra/prow/config",
    library = ":go_default_library",
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
    importpath = "k8s.io/test-infra/prow/config",
    deps = [
        "//prow/kube:go_default_library",
        "//prow/kube/labels:go_default_library",
        "//vendor/github.com/ghodss/yaml:go_default_library",
        "//vendor/github.com/sirupsen/logrus:go_default_library",
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
