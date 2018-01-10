load("@io_bazel_rules_go//go:def.bzl", "go_library")
load("@io_bazel_rules_go//proto:def.bzl", "go_proto_library")

proto_library(
    name = "modules_proto",
    srcs = ["modules_service.proto"],
    tags = ["manual"],
    visibility = ["//vendor/google.golang.org/appengine:__subpackages__"],
)

go_proto_library(
    name = "modules_go_proto",
    importpath = "google.golang.org/appengine/internal/modules",
    proto = ":modules_proto",
    tags = ["manual"],
    visibility = ["//vendor/google.golang.org/appengine:__subpackages__"],
)

go_library(
    name = "go_default_library",
    embed = [":modules_go_proto"],
    importpath = "google.golang.org/appengine/internal/modules",
    tags = ["manual"],
    visibility = ["//vendor/google.golang.org/appengine:__subpackages__"],
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
