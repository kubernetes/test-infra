load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["main.go"],
    importpath = "golang.org/x/tools/cmd/fiximports",
    visibility = ["//visibility:private"],
)

go_binary(
    name = "fiximports",
    embed = [":go_default_library"],
    importpath = "golang.org/x/tools/cmd/fiximports",
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
        "//vendor/golang.org/x/tools/cmd/fiximports/testdata/src/fruit.io/banana:all-srcs",
        "//vendor/golang.org/x/tools/cmd/fiximports/testdata/src/fruit.io/orange:all-srcs",
        "//vendor/golang.org/x/tools/cmd/fiximports/testdata/src/fruit.io/pear:all-srcs",
        "//vendor/golang.org/x/tools/cmd/fiximports/testdata/src/new.com/one:all-srcs",
        "//vendor/golang.org/x/tools/cmd/fiximports/testdata/src/old.com/one:all-srcs",
        "//vendor/golang.org/x/tools/cmd/fiximports/testdata/src/titanic.biz/bar:all-srcs",
        "//vendor/golang.org/x/tools/cmd/fiximports/testdata/src/titanic.biz/foo:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
