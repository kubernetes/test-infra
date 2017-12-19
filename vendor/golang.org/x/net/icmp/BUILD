load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "dstunreach.go",
        "echo.go",
        "endpoint.go",
        "extension.go",
        "interface.go",
        "ipv4.go",
        "ipv6.go",
        "message.go",
        "messagebody.go",
        "mpls.go",
        "multipart.go",
        "packettoobig.go",
        "paramprob.go",
        "timeexceeded.go",
    ] + select({
        "@io_bazel_rules_go//go/platform:darwin": [
            "helper_posix.go",
            "listen_posix.go",
        ],
        "@io_bazel_rules_go//go/platform:dragonfly": [
            "helper_posix.go",
            "listen_posix.go",
        ],
        "@io_bazel_rules_go//go/platform:freebsd": [
            "helper_posix.go",
            "listen_posix.go",
            "sys_freebsd.go",
        ],
        "@io_bazel_rules_go//go/platform:linux": [
            "helper_posix.go",
            "listen_posix.go",
        ],
        "@io_bazel_rules_go//go/platform:nacl": [
            "listen_stub.go",
        ],
        "@io_bazel_rules_go//go/platform:netbsd": [
            "helper_posix.go",
            "listen_posix.go",
        ],
        "@io_bazel_rules_go//go/platform:openbsd": [
            "helper_posix.go",
            "listen_posix.go",
        ],
        "@io_bazel_rules_go//go/platform:plan9": [
            "listen_stub.go",
        ],
        "@io_bazel_rules_go//go/platform:solaris": [
            "helper_posix.go",
            "listen_posix.go",
        ],
        "@io_bazel_rules_go//go/platform:windows": [
            "helper_posix.go",
            "listen_posix.go",
        ],
        "//conditions:default": [],
    }),
    importpath = "golang.org/x/net/icmp",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/golang.org/x/net/internal/iana:go_default_library",
        "//vendor/golang.org/x/net/internal/socket:go_default_library",
        "//vendor/golang.org/x/net/ipv4:go_default_library",
        "//vendor/golang.org/x/net/ipv6:go_default_library",
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
    srcs = [":package-srcs"],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
