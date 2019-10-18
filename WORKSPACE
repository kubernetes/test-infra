# gazelle:repository_macro repos.bzl%go_repositories
workspace(name = "io_k8s_test_infra")

load("//:load.bzl", "repositories")

repositories()

load("@io_k8s_repo_infra//:load.bzl", _repo_infra_repos = "repositories")

_repo_infra_repos()

load("@io_k8s_repo_infra//:repos.bzl", "configure")

configure(
    go_modules = None,
    go_version = "1.13",
    nogo = "@//:nogo_vet",
)

load("//:repos.bzl", "go_repositories")

go_repositories()

load("@io_bazel_rules_docker//repositories:repositories.bzl", _container_repositories = "repositories")

_container_repositories()

load("@io_bazel_rules_docker//repositories:deps.bzl", _container_deps = "deps")

_container_deps()

load("@io_bazel_rules_docker//go:image.bzl", _go_repositories = "repositories")

_go_repositories()

load("@io_bazel_rules_k8s//k8s:k8s.bzl", _k8s_repos = "k8s_repositories")

_k8s_repos()

load("@io_bazel_rules_k8s//k8s:k8s_go_deps.bzl", _k8s_go_repos = "deps")

_k8s_go_repos()

load("//:containers.bzl", _container_repos = "repositories")

_container_repos()

load("@io_bazel_rules_k8s//k8s:k8s.bzl", "k8s_repositories")

k8s_repositories()

load("@build_bazel_rules_nodejs//:defs.bzl", "yarn_install")

yarn_install(
    name = "npm",
    package_json = "//:package.json",
    quiet = True,
    yarn_lock = "//:yarn.lock",
)

load("@npm//:install_bazel_dependencies.bzl", "install_bazel_dependencies")

install_bazel_dependencies()

load("@npm_bazel_typescript//:index.bzl", "ts_setup_workspace")

ts_setup_workspace()

load("@io_bazel_rules_python//python:pip.bzl", "pip_import")

pip_import(
    name = "py_deps",
    requirements = "//:requirements.txt",
)

load("//:py.bzl", "python_repos")

python_repos()

load("@io_bazel_rules_jsonnet//jsonnet:jsonnet.bzl", "jsonnet_repositories")

jsonnet_repositories()
