load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = [
        "avgvar.go",
        "iterator.go",
        "llrb.go",
        "llrb-stats.go",
        "util.go",
    ],
    importpath = "github.com/petar/GoLLRB/llrb",
    visibility = ["//visibility:public"],
)

go_test(
    name = "go_default_test",
    srcs = [
        "iterator_test.go",
        "llrb_test.go",
    ],
    importpath = "github.com/petar/GoLLRB/llrb",
    library = ":go_default_library",
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
