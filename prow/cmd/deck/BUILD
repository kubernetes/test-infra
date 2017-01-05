package(default_visibility = ["//visibility:public"])

licenses(["notice"])

load(
    "@io_bazel_rules_go//go:def.bzl",
    "go_binary",
    "go_library",
    "go_test",
)

go_binary(
    name = "deck",
    library = ":go_default_library",
    tags = ["automanaged"],
)

go_test(
    name = "go_default_test",
    srcs = ["main_test.go"],
    library = ":go_default_library",
    tags = ["automanaged"],
)

go_library(
    name = "go_default_library",
    srcs = [
        "jobs.go",
        "main.go",
    ],
    tags = ["automanaged"],
    deps = [
        "//prow/kube:go_default_library",
        "//vendor:github.com/NYTimes/gziphandler",
        "//vendor:github.com/Sirupsen/logrus",
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
