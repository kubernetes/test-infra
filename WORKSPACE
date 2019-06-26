workspace(name = "io_k8s_test_infra")

load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive", "http_file")
load("@bazel_tools//tools/build_defs/repo:git.bzl", "git_repository")

http_archive(
    name = "bazel_toolchains",
    sha256 = "c6159396a571280c71d072a38147d43dcb44f78fc15976d0d47e6d0bf015458d",
    strip_prefix = "bazel-toolchains-0.26.1",
    urls = [
        "https://mirror.bazel.build/github.com/bazelbuild/bazel-toolchains/archive/0.26.1.tar.gz",
        "https://github.com/bazelbuild/bazel-toolchains/archive/0.26.1.tar.gz",
    ],
)

load("@bazel_toolchains//rules:rbe_repo.bzl", "rbe_autoconfig")

rbe_autoconfig(name = "rbe_default")

git_repository(
    name = "bazel_skylib",
    commit = "f83cb8dd6f5658bc574ccd873e25197055265d1c",
    remote = "https://github.com/bazelbuild/bazel-skylib.git",
    shallow_since = "1543273402 -0500",
    # tag = "0.6.0",
)

load("@bazel_skylib//lib:versions.bzl", "versions")

versions.check(minimum_bazel_version = "0.26.0")

# TODO(fejta): delete the com_google_protobuf rule once rules_go >= 0.18.4
# This silences the shallow_since message (and increases cache hit rate)
git_repository(
    name = "com_google_protobuf",
    commit = "582743bf40c5d3639a70f98f183914a2c0cd0680",  # v3.7.0, as of 2019-03-03
    remote = "https://github.com/protocolbuffers/protobuf",
    shallow_since = "1551387314 -0800",
)

git_repository(
    name = "io_bazel_rules_go",
    commit = "3c34e66b0507056e83bcbd9c963ab9d7e6cb049f",  # Delete com_google_protobuf please
    remote = "https://github.com/bazelbuild/rules_go.git",
    shallow_since = "1555072240 -0400",
    # tag = "0.18.3",
)

git_repository(
    name = "bazel_gazelle",
    commit = "e530fae7ce5cfda701f450ab2d7c4619b87f4df9",
    remote = "https://github.com/bazelbuild/bazel-gazelle",
    shallow_since = "1554245619 -0400",
    # tag = "latest" and matching go.mod
)

load("@io_bazel_rules_go//go:deps.bzl", "go_register_toolchains", "go_rules_dependencies")

go_rules_dependencies()

go_register_toolchains(
    go_version = "1.12.2",
    nogo = "@//:nogo_vet",
)

load("@bazel_gazelle//:deps.bzl", "gazelle_dependencies")

gazelle_dependencies()

http_archive(
    name = "io_bazel_rules_docker",
    sha256 = "aed1c249d4ec8f703edddf35cbe9dfaca0b5f5ea6e4cd9e83e99f3b0d1136c3d",
    strip_prefix = "rules_docker-0.7.0",
    urls = ["https://github.com/bazelbuild/rules_docker/archive/v0.7.0.tar.gz"],
)

load(
    "@io_bazel_rules_docker//go:image.bzl",
    _go_repositories = "repositories",
)

_go_repositories()

load(
    "@io_bazel_rules_docker//container:container.bzl",
    "container_pull",
)

container_pull(
    name = "distroless-base",
    digest = "sha256:472206d4c501691d9e72cafca4362f2adbc610fecff3dfa42e5b345f9b7d05e5",  # 2018/10/25
    registry = "gcr.io",
    repository = "distroless/base",
    tag = "latest",
)

container_pull(
    name = "alpine-base",
    digest = "sha256:bd327018b3effc802514b63cc90102bfcd92765f4486fc5abc28abf7eb9f1e4d",  # 2018/09/20
    registry = "gcr.io",
    repository = "k8s-prow/alpine",
    tag = "0.1",
)

container_pull(
    name = "gcloud-base",
    digest = "sha256:1dbdee42a553dd6a652d64df1902015ba36ef12d6c16df568a59843e410e270b",  # 2018/10/25
    registry = "gcr.io",
    repository = "cloud-builders/gcloud",
    tag = "latest",
)

container_pull(
    name = "git-base",
    digest = "sha256:01b0f83fe91b782ec7ddf1e742ab7cc9a2261894fd9ab0760ebfd39af2d6ab28",  # 2018/07/02
    registry = "gcr.io",
    repository = "k8s-prow/git",
    tag = "0.2",
)

container_pull(
    name = "python",
    digest = "sha256:0888426cc407c5ce9f2d656d776757f8fdb31795e01f60df38a5bacb697a0db0",  # 2018/10/25
    registry = "index.docker.io",
    repository = "library/python",
    tag = "2",
)

container_pull(
    name = "gcloud-go",
    digest = "sha256:0dd11e500c64b7e722ad13bc9616598a14bb0f66d9e1de4330456c646eaf237d",  # 2019/01/25
    registry = "gcr.io",
    repository = "k8s-testimages/gcloud-in-go",
    tag = "v20190125-cc5d6ecff3",
)

http_archive(
    name = "io_bazel_rules_k8s",
    sha256 = "91fef3e6054096a8947289ba0b6da3cba559ecb11c851d7bdfc9ca395b46d8d8",
    strip_prefix = "rules_k8s-0.1",
    urls = ["https://github.com/bazelbuild/rules_k8s/archive/v0.1.tar.gz"],
)

load("@io_bazel_rules_k8s//k8s:k8s.bzl", "k8s_repositories")

k8s_repositories()

git_repository(
    name = "io_k8s_repo_infra",
    commit = "4ce715fbe67d8fbed05ec2bb47a148e754100a4b",
    remote = "https://github.com/kubernetes/repo-infra.git",
    shallow_since = "1517262872 -0800",
)

# https://github.com/bazelbuild/rules_nodejs
http_archive(
    name = "build_bazel_rules_nodejs",
    sha256 = "e04a82a72146bfbca2d0575947daa60fda1878c8d3a3afe868a8ec39a6b968bb",
    urls = ["https://github.com/bazelbuild/rules_nodejs/releases/download/0.31.1/rules_nodejs-0.31.1.tar.gz"],
)

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

# Python setup
# pip_import() calls must live in WORKSPACE, otherwise we get a load() after non-load() error
git_repository(
    name = "io_bazel_rules_python",
    commit = "cc4cbf2f042695f4d1d4198c22459b3dbe7f8e43",
    remote = "https://github.com/bazelbuild/rules_python.git",
    shallow_since = "1546820050 -0500",
)

# TODO(fejta): get this to work
git_repository(
    name = "io_bazel_rules_appengine",
    commit = "fdbce051adecbb369b15260046f4f23684369efc",
    remote = "https://github.com/bazelbuild/rules_appengine.git",
    shallow_since = "1552415147 -0400",
    #tag = "0.0.8+but-this-isn't-new-enough", # Latest at https://github.com/bazelbuild/rules_appengine/releases.
)

load("@io_bazel_rules_python//python:pip.bzl", "pip_import")

pip_import(
    name = "py_deps",
    requirements = "//:requirements.txt",
)

load("//:py.bzl", "python_repos")

python_repos()

load("//:repos.bzl", "go_repositories")

go_repositories()

load("@bazel_tools//tools/build_defs/repo:git.bzl", "new_git_repository")

new_git_repository(
    name = "operator_framework_community_operators",
    build_file_content = """
exports_files([
    "upstream-community-operators/prometheus/alertmanager.crd.yaml",
    "upstream-community-operators/prometheus/prometheus.crd.yaml",
    "upstream-community-operators/prometheus/prometheusrule.crd.yaml",
    "upstream-community-operators/prometheus/servicemonitor.crd.yaml",
])
""",
    commit = "42131df7167ec0b264c892c1f3c49ba9a72142da",
    remote = "https://github.com/operator-framework/community-operators.git",
)
