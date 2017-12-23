load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = select({
        "@io_bazel_rules_go//go/platform:solaris": [
            "address.go",
            "binary.go",
            "lif.go",
            "link.go",
            "syscall.go",
        ],
        "//conditions:default": [],
    }) + select({
        "@io_bazel_rules_go//go/platform:solaris_amd64": [
            "sys_solaris_amd64.s",
            "zsys_solaris_amd64.go",
        ],
        "//conditions:default": [],
    }),
    cgo = True,
    importpath = "golang.org/x/net/lif",
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
