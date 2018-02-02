load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")
load("@io_bazel_rules_docker//go:image.bzl", "go_image")
load("@io_bazel_rules_go//proto:def.bzl", "go_proto_library")

proto_library(
    name = "state_proto",
    srcs = ["state.proto"],
)

go_proto_library(
    name = "state",
    proto = ":state_proto",
)

go_library(
    name = "go_default_library",
    srcs = ["main.go"],
    importpath = "k8s.io/test-infra/testgrid/updater",
    visibility = ["//visibility:private"],
    deps = [
        ":state",
        "//vendor/cloud.google.com/go/storage:go_default_library",
    ],
)

go_binary(
    name = "updater",
    embed = [":go_default_library"],
    importpath = "k8s.io/test-infra/testgrid/updater",
    pure = "on",
    tags = ["automanaged"],
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
