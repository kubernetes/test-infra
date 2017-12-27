load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "common.go",
        "message.go",
        "plural.go",
        "tables.go",
    ],
    importpath = "golang.org/x/text/feature/plural",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/golang.org/x/text/internal/catmsg:go_default_library",
        "//vendor/golang.org/x/text/internal/number:go_default_library",
        "//vendor/golang.org/x/text/language:go_default_library",
        "//vendor/golang.org/x/text/message/catalog:go_default_library",
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
    srcs = [":package-srcs"],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
