load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "marshal.go",
        "read.go",
        "typeinfo.go",
        "xml.go",
    ],
    importpath = "golang.org/x/net/webdav/internal/xml",
    visibility = ["//vendor/golang.org/x/net/webdav:__subpackages__"],
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
