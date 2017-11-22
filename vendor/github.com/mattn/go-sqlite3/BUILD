load("@io_bazel_rules_go//go:def.bzl", "go_library")

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
        "sqlite3_context.go",
        "sqlite3_go18.go",
        "sqlite3_load_extension.go",
        "sqlite3_other.go",
        "sqlite3_type.go",
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
        "-DSQLITE_ENABLE_RTREE -DSQLITE_THREADSAFE=1",
        "-DSQLITE_ENABLE_FTS3 -DSQLITE_ENABLE_FTS3_PARENTHESIS -DSQLITE_ENABLE_FTS4_UNICODE61",
        "-DSQLITE_TRACE_SIZE_LIMIT=15",
        "-DSQLITE_DISABLE_INTRINSIC",
        "-Wno-deprecated-declarations",
        "-Ivendor/github.com/mattn/go-sqlite3",
    ] + select({
        "@io_bazel_rules_go//go/platform:windows_amd64": [
            "-Ivendor/github.com/mattn/go-sqlite3 -fno-stack-check -fno-stack-protector -mno-stack-arg-probe",
        ],
        "//conditions:default": [],
    }),
    importpath = "github.com/mattn/go-sqlite3",
    visibility = ["//visibility:public"],
    deps = ["//vendor/golang.org/x/net/context:go_default_library"],
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
