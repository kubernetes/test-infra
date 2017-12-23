load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "golint.go",
        "import.go",
    ],
    importpath = "github.com/golang/lint/golint",
    visibility = ["//visibility:private"],
    deps = ["//vendor/github.com/golang/lint:go_default_library"],
)

go_binary(
    name = "golint",
    embed = [":go_default_library"],
    importpath = "github.com/golang/lint/golint",
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
