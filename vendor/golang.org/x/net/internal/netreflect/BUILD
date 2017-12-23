load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "socket.go",
    ] + select({
        "@io_bazel_rules_go//go/platform:android": [
            "socket_stub.go",
        ],
        "@io_bazel_rules_go//go/platform:darwin": [
            "socket_posix.go",
        ],
        "@io_bazel_rules_go//go/platform:dragonfly": [
            "socket_posix.go",
        ],
        "@io_bazel_rules_go//go/platform:freebsd": [
            "socket_posix.go",
        ],
        "@io_bazel_rules_go//go/platform:linux": [
            "socket_posix.go",
        ],
        "@io_bazel_rules_go//go/platform:nacl": [
            "socket_stub.go",
        ],
        "@io_bazel_rules_go//go/platform:netbsd": [
            "socket_posix.go",
        ],
        "@io_bazel_rules_go//go/platform:openbsd": [
            "socket_posix.go",
        ],
        "@io_bazel_rules_go//go/platform:plan9": [
            "socket_stub.go",
        ],
        "@io_bazel_rules_go//go/platform:solaris": [
            "socket_posix.go",
        ],
        "@io_bazel_rules_go//go/platform:windows": [
            "socket_posix.go",
        ],
        "//conditions:default": [],
    }),
    importpath = "golang.org/x/net/internal/netreflect",
    visibility = ["//vendor/golang.org/x/net:__subpackages__"],
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
