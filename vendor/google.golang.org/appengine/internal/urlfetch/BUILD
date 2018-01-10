load("@io_bazel_rules_go//go:def.bzl", "go_library")
load("@io_bazel_rules_go//proto:def.bzl", "go_proto_library")

proto_library(
    name = "urlfetch_proto",
    srcs = ["urlfetch_service.proto"],
    visibility = ["//vendor/google.golang.org/appengine:__subpackages__"],
)

go_proto_library(
    name = "urlfetch_go_proto",
    importpath = "google.golang.org/appengine/internal/urlfetch",
    proto = ":urlfetch_proto",
    visibility = ["//vendor/google.golang.org/appengine:__subpackages__"],
)

go_library(
    name = "go_default_library",
    embed = [":urlfetch_go_proto"],
    importpath = "google.golang.org/appengine/internal/urlfetch",
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
