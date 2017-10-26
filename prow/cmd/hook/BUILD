package(default_visibility = ["//visibility:public"])

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
    importpath = "k8s.io/test-infra/prow/cmd/hook",
    library = ":go_default_library",
)

go_test(
    name = "go_default_test",
    srcs = ["main_test.go"],
    data = [
        "//prow:configs",
    ],
    importpath = "k8s.io/test-infra/prow/cmd/hook",
    library = ":go_default_library",
    deps = ["//prow/plugins:go_default_library"],
)

go_library(
    name = "go_default_library",
    srcs = ["main.go"],
    importpath = "k8s.io/test-infra/prow/cmd/hook",
    deps = [
        "//prow/config:go_default_library",
        "//prow/git:go_default_library",
        "//prow/github:go_default_library",
        "//prow/hook:go_default_library",
        "//prow/kube:go_default_library",
        "//prow/plugins:go_default_library",
        "//prow/repoowners:go_default_library",
        "//prow/slack:go_default_library",
        "//vendor/github.com/prometheus/client_golang/prometheus:go_default_library",
        "//vendor/github.com/prometheus/client_golang/prometheus/promhttp:go_default_library",
        "//vendor/github.com/prometheus/client_golang/prometheus/push:go_default_library",
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
