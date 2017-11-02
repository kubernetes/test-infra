load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = [
        "buffer.go",
        "collations.go",
        "connection.go",
        "const.go",
        "driver.go",
        "dsn.go",
        "errors.go",
        "infile.go",
        "packets.go",
        "result.go",
        "rows.go",
        "statement.go",
        "transaction.go",
        "utils.go",
    ],
    importpath = "github.com/go-sql-driver/mysql",
    visibility = ["//visibility:public"],
)

go_test(
    name = "go_default_test",
    srcs = [
        "benchmark_test.go",
        "driver_test.go",
        "dsn_test.go",
        "errors_test.go",
        "utils_test.go",
    ],
    importpath = "github.com/go-sql-driver/mysql",
    library = ":go_default_library",
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
