load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "doc.go",
        "yacc.go",
    ],
    importpath = "golang.org/x/tools/cmd/goyacc",
    visibility = ["//visibility:private"],
)

go_binary(
    name = "goyacc",
    embed = [":go_default_library"],
    importpath = "golang.org/x/tools/cmd/goyacc",
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
        "//vendor/golang.org/x/tools/cmd/goyacc/testdata/expr:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
