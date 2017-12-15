load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "main.go",
        "main19.go",
    ],
    importpath = "golang.org/x/tools/cmd/guru/testdata/src/describe",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/lib:go_default_library",
        "//vendor/nosuchpkg:go_default_library",
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
