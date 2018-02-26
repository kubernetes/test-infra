load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "lockfile.go",
    ] + select({
        "@io_bazel_rules_go//go/platform:darwin": [
            "lockfile_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:dragonfly": [
            "lockfile_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:freebsd": [
            "lockfile_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:linux": [
            "lockfile_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:nacl": [
            "lockfile_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:netbsd": [
            "lockfile_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:openbsd": [
            "lockfile_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:solaris": [
            "lockfile_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:windows": [
            "lockfile_windows.go",
        ],
        "//conditions:default": [],
    }),
    importpath = "github.com/nightlyone/lockfile",
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
