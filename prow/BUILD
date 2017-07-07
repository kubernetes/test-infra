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
        "//prow/cmd/horologium:all-srcs",
        "//prow/cmd/mkpj:all-srcs",
        "//prow/cmd/phony:all-srcs",
        "//prow/cmd/plank:all-srcs",
        "//prow/cmd/sinker:all-srcs",
        "//prow/cmd/splice:all-srcs",
        "//prow/cmd/tot:all-srcs",
        "//prow/config:all-srcs",
        "//prow/git:all-srcs",
        "//prow/github:all-srcs",
        "//prow/hook:all-srcs",
        "//prow/jenkins:all-srcs",
        "//prow/kube:all-srcs",
        "//prow/phony:all-srcs",
        "//prow/plank:all-srcs",
        "//prow/plugins:all-srcs",
        "//prow/slack:all-srcs",
    ],
    tags = ["automanaged"],
)
