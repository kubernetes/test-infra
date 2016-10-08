package(default_visibility = ["//visibility:public"])

licenses(["notice"])

load(
    "@io_bazel_rules_go//go:def.bzl",
    "go_library",
    "go_test",
)

go_test(
    name = "go_default_test",
    srcs = [
        "github_test.go",
        "status_change_test.go",
    ],
    library = ":go_default_library",
    tags = ["automanaged"],
    deps = [
        "//mungegithub/github/testing:go_default_library",
        "//vendor:github.com/google/go-github/github",
    ],
)

go_library(
    name = "go_default_library",
    srcs = [
        "github.go",
        "status_change.go",
        "webhook.go",
    ],
    tags = ["automanaged"],
    deps = [
        "//vendor:github.com/golang/glog",
        "//vendor:github.com/google/go-github/github",
        "//vendor:github.com/gregjones/httpcache",
        "//vendor:github.com/gregjones/httpcache/diskcache",
        "//vendor:github.com/peterbourgon/diskv",
        "//vendor:github.com/spf13/cobra",
        "//vendor:golang.org/x/oauth2",
        "//vendor:k8s.io/kubernetes/pkg/util/sets",
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
        "//mungegithub/github/testing:all-srcs",
    ],
    tags = ["automanaged"],
)
