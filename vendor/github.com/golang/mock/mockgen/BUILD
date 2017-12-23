load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "mockgen.go",
        "parse.go",
        "reflect.go",
    ],
    importpath = "github.com/golang/mock/mockgen",
    visibility = ["//visibility:private"],
    deps = ["//vendor/github.com/golang/mock/mockgen/model:go_default_library"],
)

go_binary(
    name = "mockgen",
    embed = [":go_default_library"],
    importpath = "github.com/golang/mock/mockgen",
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
        "//vendor/github.com/golang/mock/mockgen/model:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
