package(default_visibility = ["//visibility:public"])

load("@io_bazel_rules_docker//docker:docker.bzl", "docker_bundle")
load("@io_bazel_rules_docker//contrib:push-all.bzl", "docker_push")

docker_bundle(
    name = "release",
    images = {
        "{STABLE_PROW_REPO}/clonerefs:{DOCKER_TAG}": "//prow/cmd/clonerefs:image",
        "{STABLE_PROW_REPO}/clonerefs:latest": "//prow/cmd/clonerefs:image",
        "{STABLE_PROW_REPO}/clonerefs:latest-{BUILD_USER}": "//prow/cmd/clonerefs:image",
        "{STABLE_PROW_REPO}/deck:{DOCKER_TAG}": "//prow/cmd/deck:image",
        "{STABLE_PROW_REPO}/deck:latest": "//prow/cmd/deck:image",
        "{STABLE_PROW_REPO}/deck:latest-{BUILD_USER}": "//prow/cmd/deck:image",
        "{STABLE_PROW_REPO}/hook:{DOCKER_TAG}": "//prow/cmd/hook:image",
        "{STABLE_PROW_REPO}/hook:latest": "//prow/cmd/hook:image",
        "{STABLE_PROW_REPO}/hook:latest-{BUILD_USER}": "//prow/cmd/hook:image",
        "{STABLE_PROW_REPO}/horologium:{DOCKER_TAG}": "//prow/cmd/horologium:image",
        "{STABLE_PROW_REPO}/horologium:latest": "//prow/cmd/horologium:image",
        "{STABLE_PROW_REPO}/horologium:latest-{BUILD_USER}": "//prow/cmd/horologium:image",
        "{STABLE_PROW_REPO}/initupload:{DOCKER_TAG}": "//prow/cmd/initupload:image",
        "{STABLE_PROW_REPO}/initupload:latest": "//prow/cmd/initupload:image",
        "{STABLE_PROW_REPO}/initupload:latest-{BUILD_USER}": "//prow/cmd/initupload:image",
        "{STABLE_PROW_REPO}/jenkins-operator:{DOCKER_TAG}": "//prow/cmd/jenkins-operator:image",
        "{STABLE_PROW_REPO}/jenkins-operator:latest": "//prow/cmd/jenkins-operator:image",
        "{STABLE_PROW_REPO}/jenkins-operator:latest-{BUILD_USER}": "//prow/cmd/jenkins-operator:image",
        "{STABLE_PROW_REPO}/plank:{DOCKER_TAG}": "//prow/cmd/plank:image",
        "{STABLE_PROW_REPO}/plank:latest": "//prow/cmd/plank:image",
        "{STABLE_PROW_REPO}/plank:latest-{BUILD_USER}": "//prow/cmd/plank:image",
        "{STABLE_PROW_REPO}/sinker:{DOCKER_TAG}": "//prow/cmd/sinker:image",
        "{STABLE_PROW_REPO}/sinker:latest": "//prow/cmd/sinker:image",
        "{STABLE_PROW_REPO}/sinker:latest-{BUILD_USER}": "//prow/cmd/sinker:image",
        "{STABLE_PROW_REPO}/splice:{DOCKER_TAG}": "//prow/cmd/splice:image",
        "{STABLE_PROW_REPO}/splice:latest": "//prow/cmd/splice:image",
        "{STABLE_PROW_REPO}/splice:latest-{BUILD_USER}": "//prow/cmd/splice:image",
        "{STABLE_PROW_REPO}/tide:{DOCKER_TAG}": "//prow/cmd/tide:image",
        "{STABLE_PROW_REPO}/tide:latest": "//prow/cmd/tide:image",
        "{STABLE_PROW_REPO}/tide:latest-{BUILD_USER}": "//prow/cmd/tide:image",
        "{STABLE_PROW_REPO}/tot:{DOCKER_TAG}": "//prow/cmd/tot:image",
        "{STABLE_PROW_REPO}/tot:latest": "//prow/cmd/tot:image",
        "{STABLE_PROW_REPO}/tot:latest-{BUILD_USER}": "//prow/cmd/tot:image",
    },
    stamp = True,
)

docker_push(
    name = "release-push",
    bundle = ":release",
)

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
        "//prow/cluster:all-srcs",
        "//prow/cmd/branchprotector:all-srcs",
        "//prow/cmd/clonerefs:all-srcs",
        "//prow/cmd/config:all-srcs",
        "//prow/cmd/deck:all-srcs",
        "//prow/cmd/hook:all-srcs",
        "//prow/cmd/horologium:all-srcs",
        "//prow/cmd/initupload:all-srcs",
        "//prow/cmd/jenkins-operator:all-srcs",
        "//prow/cmd/mkpj:all-srcs",
        "//prow/cmd/phony:all-srcs",
        "//prow/cmd/plank:all-srcs",
        "//prow/cmd/sinker:all-srcs",
        "//prow/cmd/splice:all-srcs",
        "//prow/cmd/tide:all-srcs",
        "//prow/cmd/tot:all-srcs",
        "//prow/commentpruner:all-srcs",
        "//prow/config:all-srcs",
        "//prow/cron:all-srcs",
        "//prow/genfiles:all-srcs",
        "//prow/git:all-srcs",
        "//prow/github:all-srcs",
        "//prow/hook:all-srcs",
        "//prow/jenkins:all-srcs",
        "//prow/kube:all-srcs",
        "//prow/metrics:all-srcs",
        "//prow/phony:all-srcs",
        "//prow/pjutil:all-srcs",
        "//prow/plank:all-srcs",
        "//prow/pluginhelp:all-srcs",
        "//prow/plugins:all-srcs",
        "//prow/pod-utils/clone:all-srcs",
        "//prow/pod-utils/gcs:all-srcs",
        "//prow/repoowners:all-srcs",
        "//prow/report:all-srcs",
        "//prow/slack:all-srcs",
        "//prow/tide:all-srcs",
    ],
    tags = ["automanaged"],
)
