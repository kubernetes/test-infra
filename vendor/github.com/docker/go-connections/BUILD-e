load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["doc.go"],
    importpath = "github.com/docker/go-connections",
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
        "//vendor/github.com/docker/go-connections/nat:all-srcs",
        "//vendor/github.com/docker/go-connections/sockets:all-srcs",
        "//vendor/github.com/docker/go-connections/tlsconfig:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
