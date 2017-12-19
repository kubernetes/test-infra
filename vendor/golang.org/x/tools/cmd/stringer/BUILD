load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "importer18.go",
        "importer19.go",
        "stringer.go",
    ],
    importpath = "golang.org/x/tools/cmd/stringer",
    visibility = ["//visibility:private"],
)

go_binary(
    name = "stringer",
    embed = [":go_default_library"],
    importpath = "golang.org/x/tools/cmd/stringer",
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
        "//vendor/golang.org/x/tools/cmd/stringer/testdata:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
