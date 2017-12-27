load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = select({
        "@io_bazel_rules_go//go/platform:android": [
            "stress.go",
        ],
        "@io_bazel_rules_go//go/platform:darwin": [
            "stress.go",
        ],
        "@io_bazel_rules_go//go/platform:dragonfly": [
            "stress.go",
        ],
        "@io_bazel_rules_go//go/platform:freebsd": [
            "stress.go",
        ],
        "@io_bazel_rules_go//go/platform:linux": [
            "stress.go",
        ],
        "@io_bazel_rules_go//go/platform:nacl": [
            "stress.go",
        ],
        "@io_bazel_rules_go//go/platform:netbsd": [
            "stress.go",
        ],
        "@io_bazel_rules_go//go/platform:openbsd": [
            "stress.go",
        ],
        "@io_bazel_rules_go//go/platform:solaris": [
            "stress.go",
        ],
        "@io_bazel_rules_go//go/platform:windows": [
            "stress.go",
        ],
        "//conditions:default": [],
    }),
    importpath = "golang.org/x/tools/cmd/stress",
    visibility = ["//visibility:private"],
)

go_binary(
    name = "stress",
    embed = [":go_default_library"],
    importpath = "golang.org/x/tools/cmd/stress",
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
