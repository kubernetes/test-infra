load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = [
        "compression.go",
        "diskv.go",
        "index.go",
    ],
    importpath = "github.com/peterbourgon/diskv",
    visibility = ["//visibility:public"],
    deps = ["//vendor/github.com/google/btree:go_default_library"],
)

go_test(
    name = "go_default_test",
    srcs = [
        "basic_test.go",
        "compression_test.go",
        "index_test.go",
        "issues_test.go",
        "speed_test.go",
        "stream_test.go",
    ],
    importpath = "github.com/peterbourgon/diskv",
    library = ":go_default_library",
)

go_test(
    name = "go_default_xtest",
    srcs = [
        "import_test.go",
        "keys_test.go",
    ],
    importpath = "github.com/peterbourgon/diskv_test",
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
