load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "helpers.go",
        "raw.go",
    ],
    importpath = "github.com/mailru/easyjson",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/mailru/easyjson/jlexer:go_default_library",
        "//vendor/github.com/mailru/easyjson/jwriter:go_default_library",
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
        "//vendor/github.com/mailru/easyjson/buffer:all-srcs",
        "//vendor/github.com/mailru/easyjson/jlexer:all-srcs",
        "//vendor/github.com/mailru/easyjson/jwriter:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
