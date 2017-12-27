load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "dict.go",
        "display.go",
        "lookup.go",
        "tables.go",
    ],
    importpath = "golang.org/x/text/language/display",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/golang.org/x/text/internal/format:go_default_library",
        "//vendor/golang.org/x/text/language:go_default_library",
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
