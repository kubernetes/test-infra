load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "syscall.s",
    ] + select({
        "@io_bazel_rules_go//go/platform:darwin": [
            "address.go",
            "binary.go",
            "interface.go",
            "interface_classic.go",
            "interface_multicast.go",
            "message.go",
            "route.go",
            "route_classic.go",
            "sys.go",
            "sys_darwin.go",
            "syscall.go",
            "zsys_darwin.go",
        ],
        "@io_bazel_rules_go//go/platform:dragonfly": [
            "address.go",
            "binary.go",
            "interface.go",
            "interface_announce.go",
            "interface_classic.go",
            "interface_multicast.go",
            "message.go",
            "route.go",
            "route_classic.go",
            "sys.go",
            "sys_dragonfly.go",
            "syscall.go",
            "zsys_dragonfly.go",
        ],
        "@io_bazel_rules_go//go/platform:freebsd": [
            "address.go",
            "binary.go",
            "interface.go",
            "interface_announce.go",
            "interface_freebsd.go",
            "interface_multicast.go",
            "message.go",
            "route.go",
            "route_classic.go",
            "sys.go",
            "sys_freebsd.go",
            "syscall.go",
        ],
        "@io_bazel_rules_go//go/platform:netbsd": [
            "address.go",
            "binary.go",
            "interface.go",
            "interface_announce.go",
            "interface_classic.go",
            "message.go",
            "route.go",
            "route_classic.go",
            "sys.go",
            "sys_netbsd.go",
            "syscall.go",
            "zsys_netbsd.go",
        ],
        "@io_bazel_rules_go//go/platform:openbsd": [
            "address.go",
            "binary.go",
            "interface.go",
            "interface_openbsd.go",
            "message.go",
            "route.go",
            "route_openbsd.go",
            "sys.go",
            "sys_openbsd.go",
            "syscall.go",
            "zsys_openbsd.go",
        ],
        "//conditions:default": [],
    }) + select({
        "@io_bazel_rules_go//go/platform:freebsd_386": [
            "zsys_freebsd_386.go",
        ],
        "@io_bazel_rules_go//go/platform:freebsd_amd64": [
            "zsys_freebsd_amd64.go",
        ],
        "@io_bazel_rules_go//go/platform:freebsd_arm": [
            "zsys_freebsd_arm.go",
        ],
        "//conditions:default": [],
    }),
    cgo = True,
    importpath = "golang.org/x/net/route",
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
