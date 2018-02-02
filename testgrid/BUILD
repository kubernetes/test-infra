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
        "//testgrid/cmd/config:all-srcs",
        "//testgrid/cmd/updater:all-srcs",
        "//testgrid/config:all-srcs",
        "//testgrid/updater:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
