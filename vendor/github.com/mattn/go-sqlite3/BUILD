package(default_visibility = ["//visibility:public"])

licenses(["notice"])

load(
    "@io_bazel_rules_go//go:def.bzl",
    "cgo_genrule",
    "go_library",
)

cgo_genrule(
    name = "cgo_codegen",
    srcs = [
        "backup.go",
        "callback.go",
        "error.go",
        "sqlite3.go",
        "sqlite3-binding.c",
        "sqlite3-binding.h",
        "sqlite3_load_extension.go",
        "sqlite3_other.go",
        "sqlite3ext.h",
    ],
    clinkopts = [
        "-lz",
        "-lm",
        "-lpthread",
        "-ldl",
    ],
    tags = ["automanaged"],
)

go_library(
    name = "go_default_library",
    srcs = ["doc.go"],
    library = ":cgo_codegen",
    tags = ["automanaged"],
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
)
