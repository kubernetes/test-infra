load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "decoder.go",
        "encoder.go",
        "generator.go",
    ],
    importpath = "github.com/mailru/easyjson/gen",
    visibility = ["//visibility:public"],
    deps = ["//vendor/github.com/mailru/easyjson:go_default_library"],
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
