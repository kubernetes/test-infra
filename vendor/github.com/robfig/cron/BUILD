load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = [
        "constantdelay.go",
        "cron.go",
        "doc.go",
        "parser.go",
        "spec.go",
    ],
    importpath = "github.com/robfig/cron",
    visibility = ["//visibility:public"],
)

go_test(
    name = "go_default_test",
    srcs = [
        "constantdelay_test.go",
        "cron_test.go",
        "parser_test.go",
        "spec_test.go",
    ],
    importpath = "github.com/robfig/cron",
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
