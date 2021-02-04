# gazelle:repository_macro repos.bzl%go_repositories
workspace(name = "io_k8s_test_infra")

canary_repo_infra = False  # Set to true to use the local version

canary_repo_infra and local_repository(
    name = "io_k8s_repo_infra",
    path = "../repo-infra",
)

load("//:load.bzl", "repositories")

repositories()

load("@io_k8s_repo_infra//:load.bzl", _repo_infra_repos = "repositories")

_repo_infra_repos()

load("@io_k8s_repo_infra//:repos.bzl", "configure")

configure(
    go_version = "1.15",
    nogo = "@//:nogo_vet",
)

load("//:repos.bzl", "go_repositories")

go_repositories()

load("@io_k8s_repo_infra//:repos.bzl", _repo_infra_go_repos = "go_repositories")

_repo_infra_go_repos()

load("@io_bazel_rules_docker//repositories:repositories.bzl", _container_repositories = "repositories")

_container_repositories()

load("@io_bazel_rules_docker//repositories:deps.bzl", _container_deps = "deps")

_container_deps()

load("@io_bazel_rules_docker//repositories:pip_repositories.bzl", _pip_deps = "pip_deps")

_pip_deps()

load("@io_bazel_rules_docker//go:image.bzl", _go_repositories = "repositories")

_go_repositories()

load("@io_bazel_rules_k8s//k8s:k8s.bzl", _k8s_repos = "k8s_repositories")
load("@io_bazel_rules_k8s//toolchains/kubectl:kubectl_configure.bzl", "kubectl_configure")

kubectl_configure(name = "k8s_config")

_k8s_repos()

load("@io_bazel_rules_k8s//k8s:k8s_go_deps.bzl", _k8s_go_repos = "deps")

_k8s_go_repos()

load("//:containers.bzl", _container_repos = "repositories")

_container_repos()

load("@io_bazel_rules_k8s//k8s:k8s.bzl", "k8s_repositories")

k8s_repositories()

# TODO(fejta): node repositories
load("@build_bazel_rules_nodejs//:index.bzl", "yarn_install")

yarn_install(
    name = "npm",
    # Updating yarn.lock? Set frozen_lockfile=False,
    frozen_lockfile = True,
    package_json = "//:package.json",
    quiet = True,
    yarn_lock = "//:yarn.lock",
)

load("@rules_python//python:pip.bzl", "pip_import")

pip_import(
    name = "py_deps",
    python_interpreter = "python2.7",
    requirements = "//:requirements2.txt",
)

load("@py_deps//:requirements.bzl", "pip_install")

pip_install()

pip_import(
    name = "py3_deps",
    python_interpreter = "python3",
    requirements = "//:requirements3.txt",
)

load("//:py.bzl", "python_repos")

python_repos()

load("@io_bazel_rules_jsonnet//jsonnet:jsonnet.bzl", "jsonnet_repositories")

jsonnet_repositories()
