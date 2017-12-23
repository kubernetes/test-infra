load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "doc.go",
        "format.go",
        "number.go",
        "option.go",
    ],
    importpath = "golang.org/x/text/number",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/golang.org/x/text/feature/plural:go_default_library",
        "//vendor/golang.org/x/text/internal/format:go_default_library",
        "//vendor/golang.org/x/text/internal/number:go_default_library",
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
