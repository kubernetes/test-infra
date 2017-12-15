load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "batch_cursor.gen.go",
        "bit_reader.go",
        "bool.go",
        "cache.go",
        "compact.gen.go",
        "compact.go",
        "encoding.gen.go",
        "encoding.go",
        "engine.go",
        "engine_cursor.go",
        "file_store.gen.go",
        "file_store.go",
        "float.go",
        "int.go",
        "iterator.gen.go",
        "iterator.go",
        "pools.go",
        "reader.go",
        "ring.go",
        "scheduler.go",
        "string.go",
        "timestamp.go",
        "tombstone.go",
        "wal.go",
        "writer.go",
    ] + select({
        "@io_bazel_rules_go//go/platform:android": [
            "file_unix.go",
            "mmap_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:darwin": [
            "file_unix.go",
            "mmap_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:dragonfly": [
            "file_unix.go",
            "mmap_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:freebsd": [
            "file_unix.go",
            "mmap_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:linux": [
            "file_unix.go",
            "mmap_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:nacl": [
            "file_unix.go",
            "mmap_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:netbsd": [
            "file_unix.go",
            "mmap_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:openbsd": [
            "file_unix.go",
            "mmap_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:plan9": [
            "file_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:solaris": [
            "file_unix.go",
            "mmap_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:windows": [
            "file_windows.go",
            "mmap_windows.go",
        ],
        "//conditions:default": [],
    }),
    importpath = "github.com/influxdata/influxdb/tsdb/engine/tsm1",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/cespare/xxhash:go_default_library",
        "//vendor/github.com/dgryski/go-bitstream:go_default_library",
        "//vendor/github.com/golang/snappy:go_default_library",
        "//vendor/github.com/influxdata/influxdb/models:go_default_library",
        "//vendor/github.com/influxdata/influxdb/pkg/bits:go_default_library",
        "//vendor/github.com/influxdata/influxdb/pkg/bytesutil:go_default_library",
        "//vendor/github.com/influxdata/influxdb/pkg/estimator:go_default_library",
        "//vendor/github.com/influxdata/influxdb/pkg/limiter:go_default_library",
        "//vendor/github.com/influxdata/influxdb/pkg/metrics:go_default_library",
        "//vendor/github.com/influxdata/influxdb/pkg/pool:go_default_library",
        "//vendor/github.com/influxdata/influxdb/pkg/tracing:go_default_library",
        "//vendor/github.com/influxdata/influxdb/pkg/tracing/fields:go_default_library",
        "//vendor/github.com/influxdata/influxdb/query:go_default_library",
        "//vendor/github.com/influxdata/influxdb/tsdb:go_default_library",
        "//vendor/github.com/influxdata/influxdb/tsdb/index:go_default_library",
        "//vendor/github.com/influxdata/influxdb/tsdb/index/inmem:go_default_library",
        "//vendor/github.com/influxdata/influxdb/tsdb/index/tsi1:go_default_library",
        "//vendor/github.com/influxdata/influxql:go_default_library",
        "//vendor/github.com/jwilder/encoding/simple8b:go_default_library",
        "//vendor/github.com/uber-go/zap:go_default_library",
    ] + select({
        "@io_bazel_rules_go//go/platform:android": [
            "//vendor/golang.org/x/sys/unix:go_default_library",
        ],
        "@io_bazel_rules_go//go/platform:darwin": [
            "//vendor/golang.org/x/sys/unix:go_default_library",
        ],
        "@io_bazel_rules_go//go/platform:dragonfly": [
            "//vendor/golang.org/x/sys/unix:go_default_library",
        ],
        "@io_bazel_rules_go//go/platform:freebsd": [
            "//vendor/golang.org/x/sys/unix:go_default_library",
        ],
        "@io_bazel_rules_go//go/platform:linux": [
            "//vendor/golang.org/x/sys/unix:go_default_library",
        ],
        "@io_bazel_rules_go//go/platform:nacl": [
            "//vendor/golang.org/x/sys/unix:go_default_library",
        ],
        "@io_bazel_rules_go//go/platform:netbsd": [
            "//vendor/golang.org/x/sys/unix:go_default_library",
        ],
        "@io_bazel_rules_go//go/platform:openbsd": [
            "//vendor/golang.org/x/sys/unix:go_default_library",
        ],
        "@io_bazel_rules_go//go/platform:solaris": [
            "//vendor/golang.org/x/sys/unix:go_default_library",
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
