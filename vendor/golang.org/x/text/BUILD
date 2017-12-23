load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["doc.go"],
    importpath = "golang.org/x/text",
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
        "//vendor/golang.org/x/text/cases:all-srcs",
        "//vendor/golang.org/x/text/cmd/gotext:all-srcs",
        "//vendor/golang.org/x/text/collate/build:all-srcs",
        "//vendor/golang.org/x/text/collate/tools/colcmp:all-srcs",
        "//vendor/golang.org/x/text/currency:all-srcs",
        "//vendor/golang.org/x/text/date:all-srcs",
        "//vendor/golang.org/x/text/encoding:all-srcs",
        "//vendor/golang.org/x/text/feature/plural:all-srcs",
        "//vendor/golang.org/x/text/internal:all-srcs",
        "//vendor/golang.org/x/text/language:all-srcs",
        "//vendor/golang.org/x/text/message:all-srcs",
        "//vendor/golang.org/x/text/number:all-srcs",
        "//vendor/golang.org/x/text/runes:all-srcs",
        "//vendor/golang.org/x/text/search:all-srcs",
        "//vendor/golang.org/x/text/secure:all-srcs",
        "//vendor/golang.org/x/text/transform:all-srcs",
        "//vendor/golang.org/x/text/unicode:all-srcs",
        "//vendor/golang.org/x/text/width:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
