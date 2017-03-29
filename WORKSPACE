git_repository(
    name = "io_bazel_rules_go",
    commit = "bfa3601d9ab664b448ddb4cc7e48eea511217aaf",
    remote = "https://github.com/bazelbuild/rules_go.git",
)

load("@io_bazel_rules_go//go:def.bzl", "go_repositories")

go_repositories()

git_repository(
    name = "org_pubref_rules_node",
    remote = "https://github.com/pubref/rules_node.git",
    tag = "v0.3.3",
)

load("@org_pubref_rules_node//node:rules.bzl", "node_repositories", "npm_repository")

node_repositories()

npm_repository(
    name = "npm_mocha",
    deps = {
        "mocha": "3.2.0",
    },
)

new_http_archive(
    name = "markupsafe",
    build_file = "BUILD.markupsafe",
    sha256 = "a6be69091dac236ea9c6bc7d012beab42010fa914c459791d627dad4910eb665",
    strip_prefix = "MarkupSafe-1.0/markupsafe",
    urls = ["https://pypi.python.org/packages/4d/de/32d741db316d8fdb7680822dd37001ef7a448255de9699ab4bfcbdf4172b/MarkupSafe-1.0.tar.gz"],
)

new_http_archive(
    name = "jinja2",
    build_file = "BUILD.jinja2",
    sha256 = "702a24d992f856fa8d5a7a36db6128198d0c21e1da34448ca236c42e92384825",
    strip_prefix = "Jinja2-2.9.5/jinja2",
    urls = ["https://pypi.python.org/packages/71/59/d7423bd5e7ddaf3a1ce299ab4490e9044e8dfd195420fc83a24de9e60726/Jinja2-2.9.5.tar.gz"],
)
