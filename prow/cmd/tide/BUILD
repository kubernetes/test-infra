load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["main.go"],
    visibility = ["//visibility:private"],
    deps = [
        "//prow/config:go_default_library",
        "//prow/github:go_default_library",
        "//prow/kube:go_default_library",
        "//prow/tide:go_default_library",
        "//vendor/github.com/Sirupsen/logrus:go_default_library",
    ],
)

go_binary(
    name = "tide",
    library = ":go_default_library",
    visibility = ["//visibility:public"],
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
