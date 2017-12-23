load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "command.go",
        "config.go",
        "config_command.go",
        "server.go",
    ],
    importpath = "github.com/influxdata/influxdb/cmd/influxd/run",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/BurntSushi/toml:go_default_library",
        "//vendor/github.com/influxdata/influxdb:go_default_library",
        "//vendor/github.com/influxdata/influxdb/coordinator:go_default_library",
        "//vendor/github.com/influxdata/influxdb/influxql:go_default_library",
        "//vendor/github.com/influxdata/influxdb/models:go_default_library",
        "//vendor/github.com/influxdata/influxdb/monitor:go_default_library",
        "//vendor/github.com/influxdata/influxdb/services/admin:go_default_library",
        "//vendor/github.com/influxdata/influxdb/services/collectd:go_default_library",
        "//vendor/github.com/influxdata/influxdb/services/continuous_querier:go_default_library",
        "//vendor/github.com/influxdata/influxdb/services/graphite:go_default_library",
        "//vendor/github.com/influxdata/influxdb/services/httpd:go_default_library",
        "//vendor/github.com/influxdata/influxdb/services/meta:go_default_library",
        "//vendor/github.com/influxdata/influxdb/services/opentsdb:go_default_library",
        "//vendor/github.com/influxdata/influxdb/services/precreator:go_default_library",
        "//vendor/github.com/influxdata/influxdb/services/retention:go_default_library",
        "//vendor/github.com/influxdata/influxdb/services/snapshotter:go_default_library",
        "//vendor/github.com/influxdata/influxdb/services/subscriber:go_default_library",
        "//vendor/github.com/influxdata/influxdb/services/udp:go_default_library",
        "//vendor/github.com/influxdata/influxdb/tcp:go_default_library",
        "//vendor/github.com/influxdata/influxdb/tsdb:go_default_library",
        "//vendor/github.com/influxdata/influxdb/tsdb/engine:go_default_library",
        "//vendor/github.com/influxdata/usage-client/v1:go_default_library",
        "//vendor/go.uber.org/zap:go_default_library",
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
