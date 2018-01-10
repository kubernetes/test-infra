load("@io_bazel_rules_go//go:def.bzl", "go_library")
load("@io_bazel_rules_go//proto:def.bzl", "go_proto_library")

proto_library(
    name = "datastore_proto",
    srcs = ["datastore_v3.proto"],
    visibility = ["//vendor/google.golang.org/appengine:__subpackages__"],
)

go_proto_library(
    name = "datastore_go_proto",
    importpath = "google.golang.org/appengine/internal/datastore",
    proto = ":datastore_proto",
    visibility = ["//vendor/google.golang.org/appengine:__subpackages__"],
)

go_library(
    name = "go_default_library",
    embed = [":datastore_go_proto"],
    importpath = "google.golang.org/appengine/internal/datastore",
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
