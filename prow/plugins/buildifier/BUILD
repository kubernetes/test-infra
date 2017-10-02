package(default_visibility = ["//visibility:public"])

load(
    "@io_bazel_rules_go//go:def.bzl",
    "go_library",
    "go_test",
)

go_library(
    name = "go_default_library",
    srcs = ["buildifier.go"],
    deps = [
        "//prow/genfiles:go_default_library",
        "//prow/git:go_default_library",
        "//prow/github:go_default_library",
        "//prow/plugins:go_default_library",
        "//vendor/github.com/sirupsen/logrus:go_default_library",
        "@com_github_bazelbuild_buildtools//build:go_default_library",
    ],
)

go_test(
    name = "go_default_test",
    srcs = ["buildifier_test.go"],
    library = ":go_default_library",
    deps = [
        "//prow/git/localgit:go_default_library",
        "//prow/github:go_default_library",
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
