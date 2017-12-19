load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "add.go",
        "helpers.go",
        "init.go",
        "license_agpl.go",
        "license_apache_2.go",
        "license_bsd_clause_2.go",
        "license_bsd_clause_3.go",
        "license_gpl_2.go",
        "license_gpl_3.go",
        "license_lgpl.go",
        "license_mit.go",
        "licenses.go",
        "project.go",
        "root.go",
    ],
    importpath = "github.com/spf13/cobra/cobra/cmd",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/mitchellh/go-homedir:go_default_library",
        "//vendor/github.com/spf13/cobra:go_default_library",
        "//vendor/github.com/spf13/viper:go_default_library",
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
    srcs = [":package-srcs"],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
