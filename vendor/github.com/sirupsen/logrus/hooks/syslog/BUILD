load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = select({
        "@io_bazel_rules_go//go/platform:android": [
            "syslog.go",
        ],
        "@io_bazel_rules_go//go/platform:darwin": [
            "syslog.go",
        ],
        "@io_bazel_rules_go//go/platform:dragonfly": [
            "syslog.go",
        ],
        "@io_bazel_rules_go//go/platform:freebsd": [
            "syslog.go",
        ],
        "@io_bazel_rules_go//go/platform:linux": [
            "syslog.go",
        ],
        "@io_bazel_rules_go//go/platform:netbsd": [
            "syslog.go",
        ],
        "@io_bazel_rules_go//go/platform:openbsd": [
            "syslog.go",
        ],
        "@io_bazel_rules_go//go/platform:solaris": [
            "syslog.go",
        ],
        "//conditions:default": [],
    }),
    importpath = "github.com/sirupsen/logrus/hooks/syslog",
    visibility = ["//visibility:public"],
    deps = select({
        "@io_bazel_rules_go//go/platform:android": [
            "//vendor/github.com/sirupsen/logrus:go_default_library",
        ],
        "@io_bazel_rules_go//go/platform:darwin": [
            "//vendor/github.com/sirupsen/logrus:go_default_library",
        ],
        "@io_bazel_rules_go//go/platform:dragonfly": [
            "//vendor/github.com/sirupsen/logrus:go_default_library",
        ],
        "@io_bazel_rules_go//go/platform:freebsd": [
            "//vendor/github.com/sirupsen/logrus:go_default_library",
        ],
        "@io_bazel_rules_go//go/platform:linux": [
            "//vendor/github.com/sirupsen/logrus:go_default_library",
        ],
        "@io_bazel_rules_go//go/platform:netbsd": [
            "//vendor/github.com/sirupsen/logrus:go_default_library",
        ],
        "@io_bazel_rules_go//go/platform:openbsd": [
            "//vendor/github.com/sirupsen/logrus:go_default_library",
        ],
        "@io_bazel_rules_go//go/platform:solaris": [
            "//vendor/github.com/sirupsen/logrus:go_default_library",
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
