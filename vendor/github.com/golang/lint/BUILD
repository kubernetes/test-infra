load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = ["lint.go"],
    importpath = "github.com/golang/lint",
    visibility = ["//visibility:public"],
    deps = ["//vendor/golang.org/x/tools/go/gcexportdata:go_default_library"],
)

go_test(
    name = "go_default_test",
    srcs = ["lint_test.go"],
    importpath = "github.com/golang/lint",
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
