load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "doc.go",
        "graphql.go",
        "query.go",
        "scalar.go",
    ],
    importpath = "github.com/shurcooL/graphql",
    tags = ["manual"],
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/shurcooL/go/ctxhttp:go_default_library",
        "//vendor/github.com/shurcooL/graphql/ident:go_default_library",
        "//vendor/github.com/shurcooL/graphql/internal/jsonutil:go_default_library",
    ],
)

filegroup(
    name = "package-srcs",
    srcs = glob(["**"]),
    tags = [
        "automanaged",
        "manual",
    ],
    visibility = ["//visibility:private"],
)

filegroup(
    name = "all-srcs",
    srcs = [
        ":package-srcs",
        "//vendor/github.com/shurcooL/graphql/ident:all-srcs",
        "//vendor/github.com/shurcooL/graphql/internal/jsonutil:all-srcs",
    ],
    tags = [
        "automanaged",
        "manual",
    ],
    visibility = ["//visibility:public"],
)
