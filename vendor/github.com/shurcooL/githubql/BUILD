load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = [
        "doc.go",
        "enum.go",
        "githubql.go",
        "input.go",
        "scalar.go",
    ],
    importpath = "github.com/shurcooL/githubql",
    visibility = ["//visibility:public"],
    deps = ["//vendor/github.com/shurcooL/graphql:go_default_library"],
)

go_test(
    name = "go_default_xtest",
    srcs = [
        "githubql_test.go",
        "scalar_test.go",
    ],
    importpath = "github.com/shurcooL/githubql_test",
    deps = [":go_default_library"],
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
