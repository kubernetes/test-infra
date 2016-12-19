package(default_visibility = ["//visibility:public"])

licenses(["notice"])

load(
    "@io_bazel_rules_go//go:def.bzl",
    "go_binary",
    "go_test",
)

go_binary(
    name = "fetcher",
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
        "//vendor:github.com/golang/glog",
        "//vendor:github.com/google/go-github/github",
        "//vendor:github.com/jinzhu/gorm",
        "//vendor:github.com/jinzhu/gorm/dialects/mysql",
        "//vendor:github.com/spf13/cobra",
        "//vendor:golang.org/x/oauth2",
    ],
)

go_test(
    name = "go_default_test",
    srcs = [
        "client.go",
        "client_test.go",
        "comments.go",
        "comments_test.go",
        "conversion.go",
        "conversion_test.go",
        "fetcher.go",
        "issue-events.go",
        "issue-events_test.go",
        "issues.go",
        "issues_test.go",
    ],
    tags = ["automanaged"],
    deps = [
        "//velodrome/sql:go_default_library",
        "//velodrome/sql/testing:go_default_library",
        "//vendor:github.com/golang/glog",
        "//vendor:github.com/google/go-github/github",
        "//vendor:github.com/jinzhu/gorm",
        "//vendor:github.com/jinzhu/gorm/dialects/mysql",
        "//vendor:github.com/spf13/cobra",
        "//vendor:golang.org/x/oauth2",
    ],
)
