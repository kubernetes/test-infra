load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "client.go",
        "config.go",
        "data.go",
        "errors.go",
        "query_authorizer.go",
        "write_authorizer.go",
    ] + select({
        "@io_bazel_rules_go//go/platform:android": [
            "file_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:darwin": [
            "file_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:dragonfly": [
            "file_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:freebsd": [
            "file_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:linux": [
            "file_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:nacl": [
            "file_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:netbsd": [
            "file_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:openbsd": [
            "file_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:plan9": [
            "file_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:solaris": [
            "file_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:windows": [
            "file_windows.go",
        ],
        "//conditions:default": [],
    }),
    importpath = "github.com/influxdata/influxdb/services/meta",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/gogo/protobuf/proto:go_default_library",
        "//vendor/github.com/influxdata/influxdb:go_default_library",
        "//vendor/github.com/influxdata/influxdb/models:go_default_library",
        "//vendor/github.com/influxdata/influxdb/monitor/diagnostics:go_default_library",
        "//vendor/github.com/influxdata/influxdb/query:go_default_library",
        "//vendor/github.com/influxdata/influxdb/services/meta/internal:go_default_library",
        "//vendor/github.com/influxdata/influxql:go_default_library",
        "//vendor/github.com/uber-go/zap:go_default_library",
        "//vendor/golang.org/x/crypto/bcrypt:go_default_library",
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
    srcs = [
        ":package-srcs",
        "//vendor/github.com/influxdata/influxdb/services/meta/internal:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
