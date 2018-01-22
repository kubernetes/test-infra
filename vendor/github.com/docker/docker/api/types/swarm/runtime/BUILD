load("@io_bazel_rules_go//go:def.bzl", "go_library")
load("@io_bazel_rules_go//proto:def.bzl", "go_proto_library")

proto_library(
    name = "runtime_proto",
    srcs = ["plugin.proto"],
    visibility = ["//visibility:public"],
)

go_proto_library(
    name = "runtime_go_proto",
    importpath = "github.com/docker/docker/api/types/swarm/runtime",
    proto = ":runtime_proto",
    visibility = ["//visibility:public"],
)

go_library(
    name = "go_default_library",
    srcs = ["gen.go"],
    embed = [":runtime_go_proto"],
    importpath = "github.com/docker/docker/api/types/swarm/runtime",
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
    srcs = [":package-srcs"],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
