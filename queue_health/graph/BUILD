# The aws-janitor is not properly vendored in this repo
# gazelle:exclude aws-janitor

py_test(
    name = "graph_test",
    srcs = [
        "graph.py",
        "graph_test.py",
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
    srcs = [":package-srcs"],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
