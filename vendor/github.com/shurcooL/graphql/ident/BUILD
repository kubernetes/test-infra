load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = ["ident.go"],
    importpath = "github.com/shurcooL/graphql/ident",
    visibility = ["//visibility:public"],
)

go_test(
    name = "go_default_xtest",
    srcs = ["ident_test.go"],
    importpath = "github.com/shurcooL/graphql/ident_test",
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
