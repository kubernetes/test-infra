package(default_visibility = ["//visibility:public"])

licenses(["notice"])

load(
    "@io_bazel_rules_go//go:def.bzl",
    "go_binary",
    "go_library",
    "go_test",
)

go_binary(
    name = "fetcher",
    library = ":go_default_library",
    tags = ["automanaged"],
)

go_test(
    name = "go_default_test",
    srcs = [
        "client_test.go",
        "comments_test.go",
        "conversion_test.go",
        "issue-events_test.go",
        "issues_test.go",
    ],
    library = ":go_default_library",
    tags = ["automanaged"],
    deps = [
        "//velodrome/sql:go_default_library",
        "//velodrome/sql/testing:go_default_library",
        "//vendor/github.com/google/go-github/github:go_default_library",
    ],
)

go_library(
    name = "go_default_library",
    srcs = [
        "client.go",
        "comments.go",
        "conversion.go",
        "fetcher.go",
        "issue-events.go",
        "issues.go",
    ],
    tags = ["automanaged"],
    deps = [
        "//velodrome/sql:go_default_library",
        "//vendor/github.com/golang/glog:go_default_library",
        "//vendor/github.com/google/go-github/github:go_default_library",
        "//vendor/github.com/jinzhu/gorm:go_default_library",
        "//vendor/github.com/jinzhu/gorm/dialects/mysql:go_default_library",
        "//vendor/github.com/spf13/cobra:go_default_library",
        "//vendor/golang.org/x/oauth2:go_default_library",
    ],
)

filegroup(
    name = "configs",
    srcs = glob(["*.yaml"]),
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
)
