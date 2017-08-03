package(default_visibility = ["//visibility:public"])

licenses(["notice"])

load(
    "@io_bazel_rules_go//go:def.bzl",
    "go_library",
)

go_library(
    name = "go_default_library",
    srcs = [
        "backup.go",
        "callback.go",
        "doc.go",
        "error.go",
        "sqlite3.go",
        "sqlite3-binding.c",
        "sqlite3-binding.h",
        "sqlite3_load_extension.go",
        "sqlite3_other.go",
        "sqlite3ext.h",
    ] + select({
        "@io_bazel_rules_go//go/platform:windows_amd64": [
            "sqlite3_windows.go",
        ],
        "//conditions:default": [],
    }),
    cgo = True,
    clinkopts = select({
        "@io_bazel_rules_go//go/platform:linux_amd64": [
            "-ldl",
        ],
        "@io_bazel_rules_go//go/platform:windows_amd64": [
            "-lmingwex -lmingw32",
        ],
        "//conditions:default": [],
    }),
    copts = [
        "-std=gnu99",
        "-DSQLITE_ENABLE_RTREE -DSQLITE_THREADSAFE",
        "-DSQLITE_ENABLE_FTS3 -DSQLITE_ENABLE_FTS3_PARENTHESIS -DSQLITE_ENABLE_FTS4_UNICODE61",
        "-I.",
    ] + select({
        "@io_bazel_rules_go//go/platform:windows_amd64": [
            "-I. -fno-stack-check -fno-stack-protector -mno-stack-arg-probe",
        ],
        "//conditions:default": [],
    }),
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
