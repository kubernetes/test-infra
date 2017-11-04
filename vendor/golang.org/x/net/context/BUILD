load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = [
        "context.go",
        "go17.go",
        "pre_go17.go",
    ],
    importpath = "golang.org/x/net/context",
    visibility = ["//visibility:public"],
)

go_test(
    name = "go_default_test",
    srcs = ["context_test.go"],
    importpath = "golang.org/x/net/context",
    library = ":go_default_library",
)

go_test(
    name = "go_default_xtest",
    srcs = ["withtimeout_test.go"],
    importpath = "golang.org/x/net/context_test",
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
