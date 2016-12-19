package(default_visibility = ["//visibility:public"])

licenses(["notice"])

load(
    "@io_bazel_rules_go//go:def.bzl",
    "go_binary",
)

go_binary(
    name = "token-counter",
    srcs = [
        "influx.go",
        "token-counter.go",
    ],
    tags = ["automanaged"],
    deps = [
        "//vendor:github.com/golang/glog",
        "//vendor:github.com/google/go-github/github",
        "//vendor:github.com/influxdata/influxdb/client/v2",
        "//vendor:github.com/spf13/cobra",
        "//vendor:golang.org/x/oauth2",
    ],
)
