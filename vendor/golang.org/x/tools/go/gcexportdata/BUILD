load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "gcexportdata.go",
        "importer.go",
    ],
    importpath = "golang.org/x/tools/go/gcexportdata",
    tags = ["manual"],
    visibility = ["//visibility:public"],
    deps = ["//vendor/golang.org/x/tools/go/gcimporter15:go_default_library"],
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
    srcs = [":package-srcs"],
    tags = [
        "automanaged",
        "manual",
    ],
    visibility = ["//visibility:public"],
)
