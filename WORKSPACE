git_repository(
    name = "io_bazel_rules_go",
    commit = "bfad99993ea297e85ac3606226debcc26ee54972",
    remote = "https://github.com/bazelbuild/rules_go.git",
)

load("@io_bazel_rules_go//go:def.bzl", "go_repositories")

go_repositories()

git_repository(
    name = "org_pubref_rules_node",
    commit = "bd14a465063da90f632bad46c1efbf802c339e68",
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

new_http_archive(
    name = "requests",
    build_file_content = """
py_library(
    name = "requests",
    srcs = glob(["**/*.py"]),
    visibility = ["//visibility:public"],
)
""",
    sha256 = "5722cd09762faa01276230270ff16af7acf7c5c45d623868d9ba116f15791ce8",
    strip_prefix = "requests-2.13.0/requests",
    urls = ["https://pypi.python.org/packages/16/09/37b69de7c924d318e51ece1c4ceb679bf93be9d05973bb30c35babd596e2/requests-2.13.0.tar.gz"],
)

new_http_archive(
    name = "yaml",
    build_file_content = """
py_library(
    name = "yaml",
    srcs = glob(["*.py"]),
    visibility = ["//visibility:public"],
)
""",
    sha256 = "592766c6303207a20efc445587778322d7f73b161bd994f227adaa341ba212ab",
    strip_prefix = "PyYAML-3.12/lib/yaml",
    urls = ["https://pypi.python.org/packages/4a/85/db5a2df477072b2902b0eb892feb37d88ac635d36245a72a6a69b23b383a/PyYAML-3.12.tar.gz"],
)

new_http_archive(
    name = "markupsafe",
    build_file_content = """
py_library(
    name = "markupsafe",
    srcs = glob(["*.py"]),
    visibility = ["//visibility:public"],
)
""",
    sha256 = "a6be69091dac236ea9c6bc7d012beab42010fa914c459791d627dad4910eb665",
    strip_prefix = "MarkupSafe-1.0/markupsafe",
    urls = ["https://pypi.python.org/packages/4d/de/32d741db316d8fdb7680822dd37001ef7a448255de9699ab4bfcbdf4172b/MarkupSafe-1.0.tar.gz"],
)

new_http_archive(
    name = "jinja2",
    build_file_content = """
py_library(
    name = "jinja2",
    srcs = glob(["*.py"]),
    deps = [
        "@markupsafe//:markupsafe",
    ],
    visibility = ["//visibility:public"],
)
""",
    sha256 = "702a24d992f856fa8d5a7a36db6128198d0c21e1da34448ca236c42e92384825",
    strip_prefix = "Jinja2-2.9.5/jinja2",
    urls = ["https://pypi.python.org/packages/71/59/d7423bd5e7ddaf3a1ce299ab4490e9044e8dfd195420fc83a24de9e60726/Jinja2-2.9.5.tar.gz"],
)

new_http_archive(
    name = "pylint",
    build_file_content = """
py_binary(
    name = "pylint",
    srcs = glob(["**/*.py"]),
    visibility = ["//visibility:public"],
    main = "__main__.py",
)
""",
    sha256 = "ef901a34b62ed7a734370ba5b162d890231ba8822abe88c6dda1268e2575f5f1",
    strip_prefix = "pylint-1.6.4/pylint",
    urls = ["https://pypi.python.org/packages/4e/4b/2f14a233e6c86bbfff9568d3357860573dea51be7c96eecab9471ab6ca6f/pylint-1.6.4.tar.gz"],
)

http_file(
    name = "jq",
    executable = 1,
    sha256 = "c6b3a7d7d3e7b70c6f51b706a3b90bd01833846c54d32ca32f0027f00226ff6d",
    urls = ["https://github.com/stedolan/jq/releases/download/jq-1.5/jq-linux64"],
)
