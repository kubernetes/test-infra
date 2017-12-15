load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["exechealthz.go"],
    importpath = "k8s.io/contrib/exec-healthz",
    visibility = ["//visibility:private"],
)

go_binary(
    name = "exec-healthz",
    embed = [":go_default_library"],
    importpath = "k8s.io/contrib/exec-healthz",
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
