package(default_visibility = ["//visibility:public"])

licenses(["notice"])

load(
    "@io_bazel_rules_go//go:def.bzl",
    "go_library",
    "go_test",
)

go_test(
    name = "go_default_test",
    srcs = [
        "event_test.go",
        "finder_test.go",
    ],
    library = ":go_default_library",
    tags = ["automanaged"],
    deps = ["//vendor:github.com/google/go-github/github"],
)

go_library(
    name = "go_default_library",
    srcs = [
        "event.go",
        "finder.go",
        "operators.go",
    ],
    tags = ["automanaged"],
    deps = ["//vendor:github.com/google/go-github/github"],
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
