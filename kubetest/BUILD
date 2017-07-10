package(default_visibility = ["//visibility:public"])

licenses(["notice"])

load(
    "@io_bazel_rules_go//go:def.bzl",
    "go_binary",
    "go_library",
    "go_test",
)

go_binary(
    name = "kubetest",
    library = ":go_default_library",
    tags = ["automanaged"],
)

go_library(
    name = "go_default_library",
    srcs = [
        "anywhere.go",
        "bash.go",
        "build.go",
        "e2e.go",
        "extract.go",
        "federation.go",
        "kops.go",
        "main.go",
        "none.go",
        "stage.go",
        "util.go",
    ],
    tags = ["automanaged"],
    deps = ["//boskos/client:go_default_library"],
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
        "anywhere_test.go",
        "extract_test.go",
        "util_test.go",
    ],
    library = ":go_default_library",
    tags = ["automanaged"],
)
