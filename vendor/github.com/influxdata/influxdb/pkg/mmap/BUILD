load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = select({
        "@io_bazel_rules_go//go/platform:darwin": [
            "mmap_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:dragonfly": [
            "mmap_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:freebsd": [
            "mmap_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:linux": [
            "mmap_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:nacl": [
            "mmap_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:netbsd": [
            "mmap_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:openbsd": [
            "mmap_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:solaris": [
            "mmap_solaris.go",
        ],
        "@io_bazel_rules_go//go/platform:windows": [
            "mmap_windows.go",
        ],
        "//conditions:default": [],
    }),
    importpath = "github.com/influxdata/influxdb/pkg/mmap",
    visibility = ["//visibility:public"],
    deps = select({
        "@io_bazel_rules_go//go/platform:solaris": [
            "//vendor/golang.org/x/sys/unix:go_default_library",
        ],
        "//conditions:default": [],
    }),
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
