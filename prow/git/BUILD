package(default_visibility = ["//visibility:public"])

load(
    "@io_bazel_rules_go//go:def.bzl",
    "go_library",
    "go_test",
)

go_library(
    name = "go_default_library",
    srcs = ["git.go"],
    deps = ["//vendor/github.com/Sirupsen/logrus:go_default_library"],
)

filegroup(
    name = "package-srcs",
    srcs = glob(["**"]),
    tags = ["automanaged"],
    visibility = ["//visibility:private"],
)

filegroup(
    name = "all-srcs",
    srcs = [
        ":package-srcs",
        "//prow/git/localgit:all-srcs",
    ],
    tags = ["automanaged"],
)

go_test(
    name = "go_default_xtest",
    srcs = ["git_test.go"],
    deps = ["//prow/git/localgit:go_default_library"],
)
