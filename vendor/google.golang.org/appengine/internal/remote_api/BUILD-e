load("@io_bazel_rules_go//go:def.bzl", "go_library")
load("@io_bazel_rules_go//proto:def.bzl", "go_grpc_library")

proto_library(
    name = "remote_api_proto",
    srcs = ["remote_api.proto"],
    visibility = ["//vendor/google.golang.org/appengine:__subpackages__"],
)

go_grpc_library(
    name = "remote_api_go_proto",
    importpath = "google.golang.org/appengine/internal/remote_api",
    proto = ":remote_api_proto",
    visibility = ["//vendor/google.golang.org/appengine:__subpackages__"],
)

go_library(
    name = "go_default_library",
    embed = [":remote_api_go_proto"],
    importpath = "google.golang.org/appengine/internal/remote_api",
    visibility = ["//vendor/google.golang.org/appengine:__subpackages__"],
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
