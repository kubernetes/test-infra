load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "log.go",
    ] + select({
        "@io_bazel_rules_go//go/platform:android": [
            "syslog_formatter.go",
        ],
        "@io_bazel_rules_go//go/platform:darwin": [
            "syslog_formatter.go",
        ],
        "@io_bazel_rules_go//go/platform:dragonfly": [
            "syslog_formatter.go",
        ],
        "@io_bazel_rules_go//go/platform:freebsd": [
            "syslog_formatter.go",
        ],
        "@io_bazel_rules_go//go/platform:linux": [
            "syslog_formatter.go",
        ],
        "@io_bazel_rules_go//go/platform:netbsd": [
            "syslog_formatter.go",
        ],
        "@io_bazel_rules_go//go/platform:openbsd": [
            "syslog_formatter.go",
        ],
        "@io_bazel_rules_go//go/platform:solaris": [
            "syslog_formatter.go",
        ],
        "@io_bazel_rules_go//go/platform:windows": [
            "eventlog_formatter.go",
        ],
        "//conditions:default": [],
    }),
    importpath = "github.com/prometheus/common/log",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/sirupsen/logrus:go_default_library",
        "//vendor/gopkg.in/alecthomas/kingpin.v2:go_default_library",
    ] + select({
        "@io_bazel_rules_go//go/platform:windows": [
            "//vendor/golang.org/x/sys/windows/svc/eventlog:go_default_library",
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
