load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "callername.go",
        "doc.go",
        "funcsource.go",
        "indicies.go",
    ],
    importpath = "github.com/shurcooL/go/reflectsource",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/shurcooL/go/parserutil:go_default_library",
        "//vendor/github.com/shurcooL/go/printerutil:go_default_library",
        "//vendor/github.com/shurcooL/go/reflectfind:go_default_library",
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
