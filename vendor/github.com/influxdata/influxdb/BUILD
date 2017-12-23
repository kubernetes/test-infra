load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "errors.go",
        "influxdb.go",
        "node.go",
    ],
    importpath = "github.com/influxdata/influxdb",
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
        "//vendor/github.com/influxdata/influxdb/client:all-srcs",
        "//vendor/github.com/influxdata/influxdb/cmd:all-srcs",
        "//vendor/github.com/influxdata/influxdb/coordinator:all-srcs",
        "//vendor/github.com/influxdata/influxdb/importer/v8:all-srcs",
        "//vendor/github.com/influxdata/influxdb/influxql:all-srcs",
        "//vendor/github.com/influxdata/influxdb/internal:all-srcs",
        "//vendor/github.com/influxdata/influxdb/models:all-srcs",
        "//vendor/github.com/influxdata/influxdb/monitor:all-srcs",
        "//vendor/github.com/influxdata/influxdb/pkg/deep:all-srcs",
        "//vendor/github.com/influxdata/influxdb/pkg/escape:all-srcs",
        "//vendor/github.com/influxdata/influxdb/pkg/limiter:all-srcs",
        "//vendor/github.com/influxdata/influxdb/pkg/pool:all-srcs",
        "//vendor/github.com/influxdata/influxdb/pkg/slices:all-srcs",
        "//vendor/github.com/influxdata/influxdb/services/admin:all-srcs",
        "//vendor/github.com/influxdata/influxdb/services/collectd:all-srcs",
        "//vendor/github.com/influxdata/influxdb/services/continuous_querier:all-srcs",
        "//vendor/github.com/influxdata/influxdb/services/graphite:all-srcs",
        "//vendor/github.com/influxdata/influxdb/services/httpd:all-srcs",
        "//vendor/github.com/influxdata/influxdb/services/meta:all-srcs",
        "//vendor/github.com/influxdata/influxdb/services/opentsdb:all-srcs",
        "//vendor/github.com/influxdata/influxdb/services/precreator:all-srcs",
        "//vendor/github.com/influxdata/influxdb/services/retention:all-srcs",
        "//vendor/github.com/influxdata/influxdb/services/snapshotter:all-srcs",
        "//vendor/github.com/influxdata/influxdb/services/subscriber:all-srcs",
        "//vendor/github.com/influxdata/influxdb/services/udp:all-srcs",
        "//vendor/github.com/influxdata/influxdb/stress:all-srcs",
        "//vendor/github.com/influxdata/influxdb/tcp:all-srcs",
        "//vendor/github.com/influxdata/influxdb/tests/urlgen:all-srcs",
        "//vendor/github.com/influxdata/influxdb/toml:all-srcs",
        "//vendor/github.com/influxdata/influxdb/tsdb:all-srcs",
        "//vendor/github.com/influxdata/influxdb/uuid:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
