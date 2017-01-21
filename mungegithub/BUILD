package(default_visibility = ["//visibility:public"])

licenses(["notice"])

load(
    "@io_bazel_rules_go//go:def.bzl",
    "go_binary",
    "go_library",
)

go_binary(
    name = "mungegithub",
    library = ":go_default_library",
    tags = ["automanaged"],
)

go_library(
    name = "go_default_library",
    srcs = ["mungegithub.go"],
    tags = ["automanaged"],
    deps = [
        "//mungegithub/features:go_default_library",
        "//mungegithub/github:go_default_library",
        "//mungegithub/mungers:go_default_library",
        "//mungegithub/mungers/fsm:go_default_library",
        "//mungegithub/mungers/mungerutil:go_default_library",
        "//mungegithub/reports:go_default_library",
        "//vendor:github.com/golang/glog",
        "//vendor:github.com/spf13/cobra",
        "//vendor:k8s.io/kubernetes/pkg/util/flag",
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
    srcs = [
        ":package-srcs",
        "//mungegithub/admin:all-srcs",
        "//mungegithub/example-one-off:all-srcs",
        "//mungegithub/features:all-srcs",
        "//mungegithub/github:all-srcs",
        "//mungegithub/mungers:all-srcs",
        "//mungegithub/reports:all-srcs",
        "//mungegithub/webhooks-publisher:all-srcs",
    ],
    tags = ["automanaged"],
)

filegroup(
    name = "configs",
    srcs = glob(["*.txt"]),
    visibility = ["//visibility:public"],
)
