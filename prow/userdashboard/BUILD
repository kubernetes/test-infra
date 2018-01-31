load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["userdashboard.go"],
    importpath = "k8s.io/test-infra/prow/userdashboard",
    visibility = ["//visibility:public"],
    deps = [
        "//prow/config:go_default_library",
        "//prow/gitoauth:go_default_library",
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
    visibility = ["//visibility:public"],
)
