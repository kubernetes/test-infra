load("@io_bazel_rules_go//go:def.bzl", "go_library")
load("@io_bazel_rules_docker//go:image.bzl", "go_image")
load("@io_bazel_rules_go//proto:def.bzl", "go_proto_library")

proto_library(
    name = "state_proto",
    srcs = ["state.proto"],
    visibility = ["//visibility:public"],
)

go_proto_library(
    name = "state_go_proto",
    importpath = "k8s.io/test-infra/testgrid/updater",
    proto = ":state_proto",
    visibility = ["//visibility:public"],
)

go_library(
    name = "go_default_library",
    embed = [":state_go_proto"],
    importpath = "k8s.io/test-infra/testgrid/updater",
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
