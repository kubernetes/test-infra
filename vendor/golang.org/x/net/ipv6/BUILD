load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "control.go",
        "doc.go",
        "endpoint.go",
        "header.go",
        "helper.go",
        "iana.go",
        "icmp.go",
        "payload.go",
        "sockopt.go",
        "zsys_linux_ppc.go",
    ] + select({
        "@io_bazel_rules_go//go/platform:android": [
            "bpfopt_stub.go",
            "payload_cmsg.go",
            "sockopt_ssmreq_stub.go",
        ],
        "@io_bazel_rules_go//go/platform:darwin": [
            "bpfopt_stub.go",
            "control_rfc2292_unix.go",
            "control_rfc3542_unix.go",
            "control_unix.go",
            "dgramopt_posix.go",
            "genericopt_posix.go",
            "icmp_bsd.go",
            "payload_cmsg.go",
            "sockopt_asmreq_posix.go",
            "sockopt_posix.go",
            "sockopt_ssmreq_unix.go",
            "sys_darwin.go",
            "zsys_darwin.go",
        ],
        "@io_bazel_rules_go//go/platform:dragonfly": [
            "bpfopt_stub.go",
            "control_rfc3542_unix.go",
            "control_unix.go",
            "dgramopt_posix.go",
            "genericopt_posix.go",
            "icmp_bsd.go",
            "payload_cmsg.go",
            "sockopt_asmreq_posix.go",
            "sockopt_posix.go",
            "sockopt_ssmreq_stub.go",
            "sys_bsd.go",
            "zsys_dragonfly.go",
        ],
        "@io_bazel_rules_go//go/platform:freebsd": [
            "bpfopt_stub.go",
            "control_rfc3542_unix.go",
            "control_unix.go",
            "dgramopt_posix.go",
            "genericopt_posix.go",
            "icmp_bsd.go",
            "payload_cmsg.go",
            "sockopt_asmreq_posix.go",
            "sockopt_posix.go",
            "sockopt_ssmreq_unix.go",
            "sys_freebsd.go",
        ],
        "@io_bazel_rules_go//go/platform:linux": [
            "bpfopt_linux.go",
            "control_rfc3542_unix.go",
            "control_unix.go",
            "dgramopt_posix.go",
            "genericopt_posix.go",
            "icmp_linux.go",
            "payload_cmsg.go",
            "sockopt_asmreq_posix.go",
            "sockopt_posix.go",
            "sockopt_ssmreq_unix.go",
            "sys_linux.go",
        ],
        "@io_bazel_rules_go//go/platform:nacl": [
            "bpfopt_stub.go",
            "control_stub.go",
            "dgramopt_stub.go",
            "genericopt_stub.go",
            "icmp_stub.go",
            "payload_nocmsg.go",
            "sockopt_ssmreq_stub.go",
            "sockopt_stub.go",
            "sys_stub.go",
        ],
        "@io_bazel_rules_go//go/platform:netbsd": [
            "bpfopt_stub.go",
            "control_rfc3542_unix.go",
            "control_unix.go",
            "dgramopt_posix.go",
            "genericopt_posix.go",
            "icmp_bsd.go",
            "payload_cmsg.go",
            "sockopt_asmreq_posix.go",
            "sockopt_posix.go",
            "sockopt_ssmreq_stub.go",
            "sys_bsd.go",
            "zsys_netbsd.go",
        ],
        "@io_bazel_rules_go//go/platform:openbsd": [
            "bpfopt_stub.go",
            "control_rfc3542_unix.go",
            "control_unix.go",
            "dgramopt_posix.go",
            "genericopt_posix.go",
            "icmp_bsd.go",
            "payload_cmsg.go",
            "sockopt_asmreq_posix.go",
            "sockopt_posix.go",
            "sockopt_ssmreq_stub.go",
            "sys_bsd.go",
            "zsys_openbsd.go",
        ],
        "@io_bazel_rules_go//go/platform:plan9": [
            "bpfopt_stub.go",
            "control_stub.go",
            "dgramopt_stub.go",
            "genericopt_stub.go",
            "icmp_stub.go",
            "payload_nocmsg.go",
            "sockopt_ssmreq_stub.go",
            "sockopt_stub.go",
            "sys_stub.go",
        ],
        "@io_bazel_rules_go//go/platform:solaris": [
            "bpfopt_stub.go",
            "control_rfc3542_unix.go",
            "control_unix.go",
            "dgramopt_posix.go",
            "genericopt_posix.go",
            "icmp_solaris.go",
            "payload_cmsg.go",
            "sockopt_asmreq_posix.go",
            "sockopt_posix.go",
            "sockopt_ssmreq_unix.go",
            "sys_solaris.go",
            "syscall_solaris.go",
            "zsys_solaris.go",
        ],
        "@io_bazel_rules_go//go/platform:windows": [
            "bpfopt_stub.go",
            "control_windows.go",
            "dgramopt_posix.go",
            "genericopt_posix.go",
            "icmp_windows.go",
            "payload_nocmsg.go",
            "sockopt_asmreq_posix.go",
            "sockopt_posix.go",
            "sockopt_ssmreq_stub.go",
            "sys_windows.go",
            "syscall_windows.go",
        ],
        "//conditions:default": [],
    }) + select({
        "@io_bazel_rules_go//go/platform:darwin_386": [
            "syscall_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:darwin_amd64": [
            "syscall_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:darwin_arm": [
            "syscall_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:darwin_arm64": [
            "syscall_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:dragonfly_amd64": [
            "syscall_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:freebsd_386": [
            "syscall_unix.go",
            "zsys_freebsd_386.go",
        ],
        "@io_bazel_rules_go//go/platform:freebsd_amd64": [
            "syscall_unix.go",
            "zsys_freebsd_amd64.go",
        ],
        "@io_bazel_rules_go//go/platform:freebsd_arm": [
            "syscall_unix.go",
            "zsys_freebsd_arm.go",
        ],
        "@io_bazel_rules_go//go/platform:linux_386": [
            "sys_linux_386.s",
            "syscall_linux_386.go",
            "zsys_linux_386.go",
        ],
        "@io_bazel_rules_go//go/platform:linux_amd64": [
            "syscall_unix.go",
            "zsys_linux_amd64.go",
        ],
        "@io_bazel_rules_go//go/platform:linux_arm": [
            "syscall_unix.go",
            "zsys_linux_arm.go",
        ],
        "@io_bazel_rules_go//go/platform:linux_arm64": [
            "syscall_unix.go",
            "zsys_linux_arm64.go",
        ],
        "@io_bazel_rules_go//go/platform:linux_mips": [
            "syscall_unix.go",
            "zsys_linux_mips.go",
        ],
        "@io_bazel_rules_go//go/platform:linux_mips64": [
            "syscall_unix.go",
            "zsys_linux_mips64.go",
        ],
        "@io_bazel_rules_go//go/platform:linux_mips64le": [
            "syscall_unix.go",
            "zsys_linux_mips64le.go",
        ],
        "@io_bazel_rules_go//go/platform:linux_mipsle": [
            "syscall_unix.go",
            "zsys_linux_mipsle.go",
        ],
        "@io_bazel_rules_go//go/platform:linux_ppc64": [
            "syscall_unix.go",
            "zsys_linux_ppc64.go",
        ],
        "@io_bazel_rules_go//go/platform:linux_ppc64le": [
            "syscall_unix.go",
            "zsys_linux_ppc64le.go",
        ],
        "@io_bazel_rules_go//go/platform:linux_s390x": [
            "syscall_unix.go",
            "zsys_linux_s390x.go",
        ],
        "@io_bazel_rules_go//go/platform:netbsd_386": [
            "syscall_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:netbsd_amd64": [
            "syscall_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:netbsd_arm": [
            "syscall_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:openbsd_386": [
            "syscall_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:openbsd_amd64": [
            "syscall_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:openbsd_arm": [
            "syscall_unix.go",
        ],
        "@io_bazel_rules_go//go/platform:solaris_amd64": [
            "sys_solaris_amd64.s",
        ],
        "//conditions:default": [],
    }),
    cgo = True,
    importpath = "golang.org/x/net/ipv6",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/golang.org/x/net/internal/iana:go_default_library",
        "//vendor/golang.org/x/net/internal/netreflect:go_default_library",
    ] + select({
        "@io_bazel_rules_go//go/platform:android": [
            "//vendor/golang.org/x/net/bpf:go_default_library",
        ],
        "@io_bazel_rules_go//go/platform:darwin": [
            "//vendor/golang.org/x/net/bpf:go_default_library",
        ],
        "@io_bazel_rules_go//go/platform:dragonfly": [
            "//vendor/golang.org/x/net/bpf:go_default_library",
        ],
        "@io_bazel_rules_go//go/platform:freebsd": [
            "//vendor/golang.org/x/net/bpf:go_default_library",
        ],
        "@io_bazel_rules_go//go/platform:linux": [
            "//vendor/golang.org/x/net/bpf:go_default_library",
        ],
        "@io_bazel_rules_go//go/platform:nacl": [
            "//vendor/golang.org/x/net/bpf:go_default_library",
        ],
        "@io_bazel_rules_go//go/platform:netbsd": [
            "//vendor/golang.org/x/net/bpf:go_default_library",
        ],
        "@io_bazel_rules_go//go/platform:openbsd": [
            "//vendor/golang.org/x/net/bpf:go_default_library",
        ],
        "@io_bazel_rules_go//go/platform:plan9": [
            "//vendor/golang.org/x/net/bpf:go_default_library",
        ],
        "@io_bazel_rules_go//go/platform:solaris": [
            "//vendor/golang.org/x/net/bpf:go_default_library",
        ],
        "@io_bazel_rules_go//go/platform:windows": [
            "//vendor/golang.org/x/net/bpf:go_default_library",
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
