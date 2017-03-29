git_repository(
    name = "io_bazel_rules_go",
    remote = "https://github.com/bazelbuild/rules_go.git",
    commit = "bfa3601d9ab664b448ddb4cc7e48eea511217aaf",
)
load("@io_bazel_rules_go//go:def.bzl", "go_repositories")

go_repositories()

git_repository(
    name = "org_pubref_rules_node",
    tag = "v0.3.3",
    remote = "https://github.com/pubref/rules_node.git",
)

load("@org_pubref_rules_node//node:rules.bzl", "node_repositories", "npm_repository")

node_repositories()

npm_repository(
    name = "npm_mocha",
    deps = {
        "mocha": "3.2.0",
    },
)
