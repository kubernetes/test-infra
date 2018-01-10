load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["writer.go"],
    importpath = "github.com/mailru/easyjson/jwriter",
    tags = ["manual"],
    visibility = ["//visibility:public"],
    deps = ["//vendor/github.com/mailru/easyjson/buffer:go_default_library"],
)

filegroup(
    name = "package-srcs",
    srcs = glob(["**"]),
    tags = [
        "automanaged",
        "manual",
    ],
    visibility = ["//visibility:private"],
)

filegroup(
    name = "all-srcs",
    srcs = [":package-srcs"],
    tags = [
        "automanaged",
        "manual",
    ],
    visibility = ["//visibility:public"],
)
