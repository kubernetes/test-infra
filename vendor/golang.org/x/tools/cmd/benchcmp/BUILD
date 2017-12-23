load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "benchcmp.go",
        "compare.go",
        "doc.go",
    ],
    importpath = "golang.org/x/tools/cmd/benchcmp",
    visibility = ["//visibility:private"],
    deps = ["//vendor/golang.org/x/tools/benchmark/parse:go_default_library"],
)

go_binary(
    name = "benchcmp",
    embed = [":go_default_library"],
    importpath = "golang.org/x/tools/cmd/benchcmp",
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
