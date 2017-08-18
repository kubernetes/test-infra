load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = ["tide.go"],
    visibility = ["//visibility:public"],
    deps = [
        "//prow/config:go_default_library",
        "//prow/github:go_default_library",
        "//prow/kube:go_default_library",
        "//vendor/github.com/Sirupsen/logrus:go_default_library",
        "//vendor/github.com/shurcooL/githubql:go_default_library",
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

go_test(
    name = "go_default_test",
    srcs = ["tide_test.go"],
    library = ":go_default_library",
    deps = [
        "//prow/kube:go_default_library",
        "//vendor/github.com/Sirupsen/logrus:go_default_library",
        "//vendor/github.com/shurcooL/githubql:go_default_library",
    ],
)
