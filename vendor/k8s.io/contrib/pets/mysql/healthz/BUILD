load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["main.go"],
    importpath = "k8s.io/contrib/pets/mysql/healthz",
    visibility = ["//visibility:private"],
    deps = ["//vendor/github.com/spf13/pflag:go_default_library"],
)

go_binary(
    name = "healthz",
    embed = [":go_default_library"],
    importpath = "k8s.io/contrib/pets/mysql/healthz",
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
