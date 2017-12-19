load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["podex.go"],
    importpath = "k8s.io/contrib/podex",
    visibility = ["//visibility:private"],
    deps = [
        "//vendor/github.com/ghodss/yaml:go_default_library",
        "//vendor/gopkg.in/yaml.v2:go_default_library",
    ],
)

go_binary(
    name = "podex",
    embed = [":go_default_library"],
    importpath = "k8s.io/contrib/podex",
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
