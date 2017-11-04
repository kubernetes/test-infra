load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = [
        "consistency.go",
        "inline_fnv.go",
        "inline_strconv_parse.go",
        "points.go",
        "rows.go",
        "statistic.go",
        "time.go",
    ],
    importpath = "github.com/influxdata/influxdb/models",
    visibility = ["//visibility:public"],
    deps = ["//vendor/github.com/influxdata/influxdb/pkg/escape:go_default_library"],
)

go_test(
    name = "go_default_test",
    srcs = ["inline_strconv_parse_test.go"],
    importpath = "github.com/influxdata/influxdb/models",
    library = ":go_default_library",
)

go_test(
    name = "go_default_xtest",
    srcs = [
        "inline_fnv_test.go",
        "points_test.go",
        "statistic_test.go",
    ],
    importpath = "github.com/influxdata/influxdb/models_test",
    deps = [":go_default_library"],
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
