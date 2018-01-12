load("@io_bazel_rules_go//go:def.bzl", "go_library")
load("@io_bazel_rules_go//proto:def.bzl", "go_grpc_library")

proto_library(
    name = "app_identity_proto",
    srcs = ["app_identity_service.proto"],
    visibility = ["//vendor/google.golang.org/appengine:__subpackages__"],
)

go_grpc_library(
    name = "app_identity_go_proto",
    importpath = "google.golang.org/appengine/internal/app_identity",
    proto = ":app_identity_proto",
    visibility = ["//vendor/google.golang.org/appengine:__subpackages__"],
)

go_library(
    name = "go_default_library",
    embed = [":app_identity_go_proto"],
    importpath = "google.golang.org/appengine/internal/app_identity",
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
