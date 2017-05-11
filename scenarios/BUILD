package(default_visibility = ["//visibility:public"])

filegroup(
    name = "scenarios",
    srcs = glob(["*.py"]),
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

py_test(
    name = "kubernetes_e2e_test",
    srcs = [
        "kubernetes_e2e.py",
        "kubernetes_e2e_test.py",
    ],
)

py_test(
    name = "bigquery_test",
    srcs = [
        "bigquery.py",
        "bigquery_test.py",
    ],
    args = ["--jq=$(location @jq//file)"],
    data = [
        "//metrics:configs",
        "@jq//file",
    ],
)
