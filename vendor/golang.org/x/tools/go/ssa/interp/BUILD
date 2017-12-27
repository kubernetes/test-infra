load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "external.go",
        "interp.go",
        "map.go",
        "ops.go",
        "reflect.go",
        "value.go",
    ] + select({
        "@io_bazel_rules_go//go/platform:darwin": [
            "external_darwin.go",
            "external_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:linux": [
            "external_unix.go",
        ],
        "//conditions:default": [],
    }),
    importpath = "golang.org/x/tools/go/ssa/interp",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/golang.org/x/tools/go/ssa:go_default_library",
        "//vendor/golang.org/x/tools/go/types/typeutil:go_default_library",
    ],
)

filegroup(
    name = "package-srcs",
    srcs = glob(["**"]),
    tags = ["automanaged"],
    visibility = ["//visibility:private"],
)

filegroup(
    name = "all-srcs",
    srcs = [
        ":package-srcs",
        "//vendor/golang.org/x/tools/go/ssa/interp/testdata:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
