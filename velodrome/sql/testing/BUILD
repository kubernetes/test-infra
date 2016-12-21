package(default_visibility = ["//visibility:public"])

licenses(["notice"])

load(
    "@io_bazel_rules_go//go:def.bzl",
    "go_library",
    "go_test",
)

go_test(
    name = "go_default_test",
    srcs = ["sqlite_test.go"],
    library = ":go_default_library",
    tags = ["automanaged"],
    deps = ["//velodrome/sql:go_default_library"],
)

go_library(
    name = "go_default_library",
    srcs = ["sqlite.go"],
    tags = ["automanaged"],
    deps = [
        "//velodrome/sql:go_default_library",
        "//vendor:github.com/jinzhu/gorm",
        "//vendor:github.com/jinzhu/gorm/dialects/sqlite",
    ],
)
