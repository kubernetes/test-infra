load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["purell.go"],
    importpath = "github.com/PuerkitoBio/purell",
    tags = ["manual"],
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/PuerkitoBio/urlesc:go_default_library",
        "//vendor/golang.org/x/net/idna:go_default_library",
        "//vendor/golang.org/x/text/unicode/norm:go_default_library",
        "//vendor/golang.org/x/text/width:go_default_library",
    ],
)

filegroup(
    name = "package-srcs",
    srcs = glob(["**"]),
    tags = [
        "automanaged",
        "manual",
    ],
    visibility = ["//visibility:private"],
)

filegroup(
    name = "all-srcs",
    srcs = [":package-srcs"],
    tags = [
        "automanaged",
        "manual",
    ],
    visibility = ["//visibility:public"],
)
