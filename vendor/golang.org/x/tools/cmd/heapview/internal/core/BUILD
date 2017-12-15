load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "raw.go",
    ] + select({
        "@io_bazel_rules_go//go/platform:android": [
            "mmapfile_other.go",
        ],
        "@io_bazel_rules_go//go/platform:darwin": [
            "mmapfile.go",
        ],
        "@io_bazel_rules_go//go/platform:dragonfly": [
            "mmapfile_other.go",
        ],
        "@io_bazel_rules_go//go/platform:freebsd": [
            "mmapfile_other.go",
        ],
        "@io_bazel_rules_go//go/platform:linux": [
            "mmapfile.go",
        ],
        "@io_bazel_rules_go//go/platform:nacl": [
            "mmapfile_other.go",
        ],
        "@io_bazel_rules_go//go/platform:netbsd": [
            "mmapfile_other.go",
        ],
        "@io_bazel_rules_go//go/platform:openbsd": [
            "mmapfile_other.go",
        ],
        "@io_bazel_rules_go//go/platform:plan9": [
            "mmapfile_other.go",
        ],
        "@io_bazel_rules_go//go/platform:solaris": [
            "mmapfile_other.go",
        ],
        "@io_bazel_rules_go//go/platform:windows": [
            "mmapfile_other.go",
        ],
        "//conditions:default": [],
    }),
    importpath = "golang.org/x/tools/cmd/heapview/internal/core",
    visibility = ["//vendor/golang.org/x/tools/cmd/heapview:__subpackages__"],
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
