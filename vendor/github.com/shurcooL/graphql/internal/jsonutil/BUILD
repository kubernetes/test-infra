load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = ["graphql.go"],
    importpath = "github.com/shurcooL/graphql/internal/jsonutil",
    visibility = ["//vendor/github.com/shurcooL/graphql:__subpackages__"],
)

go_test(
    name = "go_default_xtest",
    srcs = [
        "benchmark_test.go",
        "graphql_test.go",
    ],
    importpath = "github.com/shurcooL/graphql/internal/jsonutil_test",
    deps = [
        ":go_default_library",
        "//vendor/github.com/shurcooL/graphql:go_default_library",
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
