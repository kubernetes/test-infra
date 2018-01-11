py_test(
    name = "bootstrap_test",
    srcs = [
        "bootstrap.py",
        "bootstrap_test.py",
    ],
    data = [
        "//jenkins/job-configs:configs",
        "//jenkins/job-configs/kubernetes-jenkins:configs",
        "//jenkins/job-configs/kubernetes-jenkins-pull:configs",
        "//jobs",
        "//prow:config.yaml",
    ],
    deps = ["@yaml"],
)

filegroup(
    name = "package-srcs",
    srcs = glob(["**"]),
    tags = ["automanaged"],
    visibility = ["//visibility:private"],
)

filegroup(
    name = "configs",
    srcs = glob([
        "**/*.yaml",
    ]),
    visibility = ["//visibility:public"],
)

filegroup(
    name = "all-srcs",
    srcs = [
        ":package-srcs",
        "//jenkins/aws_janitor:all-srcs",
        "//jenkins/job-configs:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
