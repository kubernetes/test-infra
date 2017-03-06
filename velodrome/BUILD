package(default_visibility = ["//visibility:public"])

sh_test(
    name = "test_config",
    srcs = [
        "test_config.sh",
    ],
    data = [
        ":config.py",
        ":configs",
    ],
    tags = ["manual"],
)

filegroup(
    name = "configs",
    srcs = glob(["**/*.yaml"]) + [
        "//velodrome/fetcher:configs",
        "//velodrome/sql:configs",
        "//velodrome/token-counter:configs",
        "//velodrome/transform:configs",
    ],
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
        "//velodrome/fetcher:all-srcs",
        "//velodrome/sql:all-srcs",
        "//velodrome/token-counter:all-srcs",
        "//velodrome/transform:all-srcs",
    ],
)
