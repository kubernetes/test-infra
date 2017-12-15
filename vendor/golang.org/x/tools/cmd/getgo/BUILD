load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = select({
        "@io_bazel_rules_go//go/platform:android": [
            "download.go",
            "main.go",
            "path.go",
            "steps.go",
            "system.go",
        ],
        "@io_bazel_rules_go//go/platform:darwin": [
            "download.go",
            "main.go",
            "path.go",
            "steps.go",
            "system.go",
            "system_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:dragonfly": [
            "download.go",
            "main.go",
            "path.go",
            "steps.go",
            "system.go",
            "system_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:freebsd": [
            "download.go",
            "main.go",
            "path.go",
            "steps.go",
            "system.go",
            "system_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:linux": [
            "download.go",
            "main.go",
            "path.go",
            "steps.go",
            "system.go",
            "system_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:nacl": [
            "download.go",
            "main.go",
            "path.go",
            "steps.go",
            "system.go",
            "system_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:netbsd": [
            "download.go",
            "main.go",
            "path.go",
            "steps.go",
            "system.go",
            "system_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:openbsd": [
            "download.go",
            "main.go",
            "path.go",
            "steps.go",
            "system.go",
            "system_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:solaris": [
            "download.go",
            "main.go",
            "path.go",
            "steps.go",
            "system.go",
            "system_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:windows": [
            "download.go",
            "main.go",
            "path.go",
            "steps.go",
            "system.go",
            "system_windows.go",
        ],
        "//conditions:default": [],
    }),
    importpath = "golang.org/x/tools/cmd/getgo",
    visibility = ["//visibility:private"],
)

go_binary(
    name = "getgo",
    embed = [":go_default_library"],
    importpath = "golang.org/x/tools/cmd/getgo",
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
    srcs = [
        ":package-srcs",
        "//vendor/golang.org/x/tools/cmd/getgo/server:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
