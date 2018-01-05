load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["main.go"],
    importpath = "k8s.io/test-infra/experiment/audit-owners",
    visibility = ["//visibility:private"],
    deps = [
        "//prow/github:go_default_library",
        "//vendor/github.com/ghodss/yaml:go_default_library",
    ],
)

go_binary(
    name = "audit-owners",
    embed = [":go_default_library"],
    importpath = "k8s.io/test-infra/experiment/audit-owners",
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
