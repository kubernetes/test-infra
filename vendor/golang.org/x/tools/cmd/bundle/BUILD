load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["main.go"],
    importpath = "golang.org/x/tools/cmd/bundle",
    visibility = ["//visibility:private"],
    deps = ["//vendor/golang.org/x/tools/go/loader:go_default_library"],
)

go_binary(
    name = "bundle",
    embed = [":go_default_library"],
    importpath = "golang.org/x/tools/cmd/bundle",
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
    srcs = [
        ":package-srcs",
        "//vendor/golang.org/x/tools/cmd/bundle/testdata/src/domain.name/importdecl:all-srcs",
        "//vendor/golang.org/x/tools/cmd/bundle/testdata/src/initial:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
