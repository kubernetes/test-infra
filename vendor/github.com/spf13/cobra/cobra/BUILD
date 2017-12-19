load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["main.go"],
    importpath = "github.com/spf13/cobra/cobra",
    visibility = ["//visibility:private"],
    deps = ["//vendor/github.com/spf13/cobra/cobra/cmd:go_default_library"],
)

go_binary(
    name = "cobra",
    embed = [":go_default_library"],
    importpath = "github.com/spf13/cobra/cobra",
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
    srcs = [
        ":package-srcs",
        "//vendor/github.com/spf13/cobra/cobra/cmd:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
