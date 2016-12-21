py_test(
    name = "bootstrap_test",
    srcs = [
        "bootstrap_test.py",
        "bootstrap.py",
    ],
    data = [
        "//jenkins/job-configs/kubernetes-jenkins:configs",
        "//jenkins/job-configs/kubernetes-jenkins-pull:configs",
        "//jenkins/job-configs:configs",
        "dockerized-e2e-runner.sh",
    ],
    tags = ["manual"],
)
