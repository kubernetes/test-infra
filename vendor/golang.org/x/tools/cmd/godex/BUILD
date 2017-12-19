load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "doc.go",
        "gc.go",
        "gccgo.go",
        "godex.go",
        "isAlias18.go",
        "isAlias19.go",
        "print.go",
        "source.go",
        "writetype.go",
    ],
    importpath = "golang.org/x/tools/cmd/godex",
    visibility = ["//visibility:private"],
)

go_binary(
    name = "godex",
    embed = [":go_default_library"],
    importpath = "golang.org/x/tools/cmd/godex",
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
