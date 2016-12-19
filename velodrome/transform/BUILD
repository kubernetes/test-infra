package(default_visibility = ["//visibility:public"])

licenses(["notice"])

load(
    "@io_bazel_rules_go//go:def.bzl",
    "go_binary",
    "go_test",
)

go_binary(
    name = "transform",
    srcs = [
        "fetcher.go",
        "first_comment.go",
        "influx.go",
        "lgtm_to_merge.go",
        "open_close.go",
        "plugins.go",
        "prstats.go",
        "rebase.go",
        "transform.go",
    ],
    tags = ["automanaged"],
    deps = [
        "//velodrome/sql:go_default_library",
        "//vendor:github.com/golang/glog",
        "//vendor:github.com/influxdata/influxdb/client/v2",
        "//vendor:github.com/jinzhu/gorm",
        "//vendor:github.com/jinzhu/gorm/dialects/mysql",
        "//vendor:github.com/spf13/cobra",
    ],
)

go_test(
    name = "go_default_test",
    srcs = [
        "fetcher.go",
        "fetcher_test.go",
        "first_comment.go",
        "influx.go",
        "lgtm_to_merge.go",
        "open_close.go",
        "plugins.go",
        "plugins_mock_test.go",
        "plugins_test.go",
        "prstats.go",
        "rebase.go",
        "transform.go",
    ],
    tags = ["automanaged"],
    deps = [
        "//velodrome/sql:go_default_library",
        "//velodrome/sql/testing:go_default_library",
        "//vendor:github.com/golang/glog",
        "//vendor:github.com/golang/mock/gomock",
        "//vendor:github.com/influxdata/influxdb/client/v2",
        "//vendor:github.com/jinzhu/gorm",
        "//vendor:github.com/jinzhu/gorm/dialects/mysql",
        "//vendor:github.com/spf13/cobra",
    ],
)
