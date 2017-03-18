package(default_visibility = ["//visibility:public"])

licenses(["notice"])

load(
    "@io_bazel_rules_go//go:def.bzl",
    "go_library",
    "go_test",
)

go_library(
    name = "go_default_library",
    srcs = [
        "bundle.go",
        "events.go",
        "plugin.go",
        "states.go",
        "type_filter_wrapper.go",
    ],
    tags = ["automanaged"],
    deps = [
        "//velodrome/sql:go_default_library",
        "//vendor:github.com/spf13/cobra",
    ],
)

filegroup(
    name = "configs",
    srcs = glob(["*.yaml"]),
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
)

go_test(
    name = "go_default_test",
    srcs = [
        "bundle_test.go",
        "events_test.go",
        "plugin_mock_test.go",
        "states_test.go",
        "type_filter_wrapper_test.go",
    ],
    library = ":go_default_library",
    tags = ["automanaged"],
    deps = [
        "//velodrome/sql:go_default_library",
        "//vendor:github.com/golang/mock/gomock",
    ],
)
