load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "boundmeth.go",
        "callstack.go",
        "complit.go",
        "coverage.go",
        "defer.go",
        "fieldprom.go",
        "ifaceconv.go",
        "ifaceprom.go",
        "initorder.go",
        "methprom.go",
        "mrvchain.go",
        "range.go",
        "recover.go",
        "reflect.go",
        "static.go",
    ],
    importpath = "golang.org/x/tools/go/ssa/interp/testdata",
    visibility = ["//visibility:private"],
)

go_binary(
    name = "testdata",
    embed = [":go_default_library"],
    importpath = "golang.org/x/tools/go/ssa/interp/testdata",
    visibility = ["//visibility:public"],
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
