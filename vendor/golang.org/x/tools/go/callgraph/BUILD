load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "callgraph.go",
        "util.go",
    ],
    importpath = "golang.org/x/tools/go/callgraph",
    visibility = ["//visibility:public"],
    deps = ["//vendor/golang.org/x/tools/go/ssa:go_default_library"],
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
        "//vendor/golang.org/x/tools/go/callgraph/cha:all-srcs",
        "//vendor/golang.org/x/tools/go/callgraph/rta:all-srcs",
        "//vendor/golang.org/x/tools/go/callgraph/static:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
