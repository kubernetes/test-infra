load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "doc.go",
    ] + select({
        "@io_bazel_rules_go//go/platform:android": [
            "trash.go",
        ],
        "@io_bazel_rules_go//go/platform:darwin": [
            "trash_darwin.go",
        ],
        "@io_bazel_rules_go//go/platform:dragonfly": [
            "trash.go",
        ],
        "@io_bazel_rules_go//go/platform:freebsd": [
            "trash.go",
        ],
        "@io_bazel_rules_go//go/platform:linux": [
            "trash.go",
        ],
        "@io_bazel_rules_go//go/platform:nacl": [
            "trash.go",
        ],
        "@io_bazel_rules_go//go/platform:netbsd": [
            "trash.go",
        ],
        "@io_bazel_rules_go//go/platform:openbsd": [
            "trash.go",
        ],
        "@io_bazel_rules_go//go/platform:plan9": [
            "trash.go",
        ],
        "@io_bazel_rules_go//go/platform:solaris": [
            "trash.go",
        ],
        "@io_bazel_rules_go//go/platform:windows": [
            "trash.go",
        ],
        "//conditions:default": [],
    }),
    importpath = "github.com/shurcooL/go/trash",
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
