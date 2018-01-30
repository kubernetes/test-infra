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
    srcs = [
        ":package-srcs",
        "//queue_health/graph/aws-janitor:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
