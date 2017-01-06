py_test(
    name = "bootstrap_test",
    srcs = [
        "bootstrap.py",
        "bootstrap_test.py",
    ],
    data = [
        "dockerized-e2e-runner.sh",
        "//jenkins/job-configs:configs",
        "//jenkins/job-configs/kubernetes-jenkins:configs",
        "//jenkins/job-configs/kubernetes-jenkins-pull:configs",
        "//jobs",
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
        "//jenkins/job-configs:all-srcs",
        "//jenkins/test_history:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
