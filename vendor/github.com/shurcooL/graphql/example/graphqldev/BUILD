load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["main.go"],
    importpath = "github.com/shurcooL/graphql/example/graphqldev",
    visibility = ["//visibility:private"],
    deps = [
        "//vendor/github.com/neelance/graphql-go:go_default_library",
        "//vendor/github.com/neelance/graphql-go/example/starwars:go_default_library",
        "//vendor/github.com/neelance/graphql-go/relay:go_default_library",
        "//vendor/github.com/shurcooL/graphql:go_default_library",
    ],
)

go_binary(
    name = "graphqldev",
    embed = [":go_default_library"],
    importpath = "github.com/shurcooL/graphql/example/graphqldev",
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
