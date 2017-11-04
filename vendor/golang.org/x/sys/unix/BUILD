load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = [
        "asm.s",
        "env_unset.go",
    ] + select({
        "@io_bazel_rules_go//go/platform:darwin_amd64": [
            "asm_darwin_amd64.s",
            "constants.go",
            "env_unix.go",
            "flock.go",
            "race0.go",
            "sockcmsg_unix.go",
            "str.go",
            "syscall.go",
            "syscall_bsd.go",
            "syscall_darwin.go",
            "syscall_darwin_amd64.go",
            "syscall_unix.go",
            "zerrors_darwin_amd64.go",
            "zsyscall_darwin_amd64.go",
            "zsysnum_darwin_amd64.go",
            "ztypes_darwin_amd64.go",
        ],
        "@io_bazel_rules_go//go/platform:linux_amd64": [
            "asm_linux_amd64.s",
            "bluetooth_linux.go",
            "constants.go",
            "env_unix.go",
            "flock.go",
            "race0.go",
            "sockcmsg_linux.go",
            "sockcmsg_unix.go",
            "str.go",
            "syscall.go",
            "syscall_linux.go",
            "syscall_linux_amd64.go",
            "syscall_unix.go",
            "zerrors_linux_amd64.go",
            "zsyscall_linux_amd64.go",
            "zsysnum_linux_amd64.go",
            "ztypes_linux_amd64.go",
        ],
        "//conditions:default": [],
    }),
    cgo = True,
    importpath = "golang.org/x/sys/unix",
    visibility = ["//visibility:public"],
)

go_test(
    name = "go_default_test",
    srcs = select({
        "@io_bazel_rules_go//go/platform:darwin_amd64": [
            "export_test.go",
        ],
        "@io_bazel_rules_go//go/platform:linux_amd64": [
            "export_test.go",
        ],
        "//conditions:default": [],
    }),
    importpath = "golang.org/x/sys/unix",
    library = ":go_default_library",
)

go_test(
    name = "go_default_xtest",
    srcs = select({
        "@io_bazel_rules_go//go/platform:darwin_amd64": [
            "mmap_unix_test.go",
            "syscall_bsd_test.go",
            "syscall_test.go",
            "syscall_unix_test.go",
        ],
        "@io_bazel_rules_go//go/platform:linux_amd64": [
            "creds_test.go",
            "mmap_unix_test.go",
            "syscall_linux_test.go",
            "syscall_test.go",
            "syscall_unix_test.go",
        ],
        "//conditions:default": [],
    }),
    importpath = "golang.org/x/sys/unix_test",
    deps = select({
        "@io_bazel_rules_go//go/platform:darwin_amd64": [
            ":go_default_library",
        ],
        "@io_bazel_rules_go//go/platform:linux_amd64": [
            ":go_default_library",
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
    visibility = ["//visibility:public"],
)
