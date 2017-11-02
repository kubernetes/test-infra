load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = [
        "doc.go",
        "fs.go",
        "ipvs.go",
        "mdstat.go",
        "proc.go",
        "proc_io.go",
        "proc_limits.go",
        "proc_stat.go",
        "stat.go",
    ],
    importpath = "github.com/prometheus/procfs",
    visibility = ["//visibility:public"],
)

go_test(
    name = "go_default_test",
    srcs = [
        "fs_test.go",
        "ipvs_test.go",
        "mdstat_test.go",
        "proc_io_test.go",
        "proc_limits_test.go",
        "proc_stat_test.go",
        "proc_test.go",
        "stat_test.go",
    ],
    importpath = "github.com/prometheus/procfs",
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
