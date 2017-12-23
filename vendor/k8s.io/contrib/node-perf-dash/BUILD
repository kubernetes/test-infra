load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "data.go",
        "downloader.go",
        "kubelet-parser.go",
        "node-perf-dash.go",
        "parser.go",
    ],
    importpath = "k8s.io/contrib/node-perf-dash",
    visibility = ["//visibility:private"],
    deps = [
        "//vendor/k8s.io/contrib/test-utils/utils:go_default_library",
        "//vendor/k8s.io/kubernetes/test/e2e/perftype:go_default_library",
    ],
)

go_binary(
    name = "node-perf-dash",
    embed = [":go_default_library"],
    importpath = "k8s.io/contrib/node-perf-dash",
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
