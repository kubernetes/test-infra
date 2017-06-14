package(default_visibility = ["//visibility:public"])

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

py_test(
    name = "bigquery_test",
    srcs = [
        "bigquery.py",
        "bigquery_test.py",
    ],
    args = ["--jq=$(location @jq//file)"],
    data = [
        ":all-srcs",
        "@jq//file",
    ],
    deps = [
        "@yaml//:yaml",
    ],
)

py_test(
    name = "influxdb_test",
    srcs = [
        "influxdb.py",
        "influxdb_test.py",
    ],
)
