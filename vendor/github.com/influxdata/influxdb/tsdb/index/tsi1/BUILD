load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "doc.go",
        "file_set.go",
        "index.go",
        "index_file.go",
        "index_files.go",
        "log_file.go",
        "measurement_block.go",
        "series_block.go",
        "tag_block.go",
        "tsi1.go",
    ],
    importpath = "github.com/influxdata/influxdb/tsdb/index/tsi1",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/influxdata/influxdb/models:go_default_library",
        "//vendor/github.com/influxdata/influxdb/pkg/bloom:go_default_library",
        "//vendor/github.com/influxdata/influxdb/pkg/bytesutil:go_default_library",
        "//vendor/github.com/influxdata/influxdb/pkg/estimator:go_default_library",
        "//vendor/github.com/influxdata/influxdb/pkg/estimator/hll:go_default_library",
        "//vendor/github.com/influxdata/influxdb/pkg/mmap:go_default_library",
        "//vendor/github.com/influxdata/influxdb/pkg/rhh:go_default_library",
        "//vendor/github.com/influxdata/influxdb/query:go_default_library",
        "//vendor/github.com/influxdata/influxdb/tsdb:go_default_library",
        "//vendor/github.com/influxdata/influxql:go_default_library",
        "//vendor/github.com/uber-go/zap:go_default_library",
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
