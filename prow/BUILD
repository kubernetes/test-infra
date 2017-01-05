package(default_visibility = ["//visibility:public"])

filegroup(
    name = "configs",
    srcs = glob(["*.yaml"]),
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
        "//prow/cmd/deck:all-srcs",
        "//prow/cmd/hook:all-srcs",
        "//prow/cmd/line:all-srcs",
        "//prow/cmd/marque:all-srcs",
        "//prow/cmd/phony:all-srcs",
        "//prow/cmd/sinker:all-srcs",
        "//prow/cmd/splice:all-srcs",
        "//prow/github:all-srcs",
        "//prow/jenkins:all-srcs",
        "//prow/jobs:all-srcs",
        "//prow/kube:all-srcs",
        "//prow/line:all-srcs",
        "//prow/plugins:all-srcs",
    ],
    tags = ["automanaged"],
)
