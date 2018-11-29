workspace(name = "test_infra")

load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive", "http_file")
load("@bazel_tools//tools/build_defs/repo:git.bzl", "git_repository")

git_repository(
    name = "bazel_skylib",
    remote = "https://github.com/bazelbuild/bazel-skylib.git",
    tag = "0.5.0",
)

load("@bazel_skylib//:lib.bzl", "versions")

versions.check(minimum_bazel_version = "0.18.0")

http_archive(
    name = "io_bazel_rules_go",
    sha256 = "f87fa87475ea107b3c69196f39c82b7bbf58fe27c62a338684c20ca17d1d8613",
    urls = ["https://github.com/bazelbuild/rules_go/releases/download/0.16.2/rules_go-0.16.2.tar.gz"],
)

load("@io_bazel_rules_go//go:def.bzl", "go_register_toolchains", "go_rules_dependencies")

go_rules_dependencies()

go_register_toolchains(go_version = "1.11.2")

http_archive(
    name = "io_bazel_rules_docker",
    sha256 = "5235045774d2f40f37331636378f21fe11f69906c0386a790c5987a09211c3c4",
    strip_prefix = "rules_docker-8010a50ef03d1e13f1bebabfc625478da075fa60",
    urls = ["https://github.com/bazelbuild/rules_docker/archive/8010a50ef03d1e13f1bebabfc625478da075fa60.tar.gz"],
)

load(
    "@io_bazel_rules_docker//go:image.bzl",
    _go_repositories = "repositories",
)

_go_repositories()

load("@io_bazel_rules_docker//docker:docker.bzl", "docker_pull", "docker_repositories")

docker_repositories()

docker_pull(
    name = "distroless-base",
    digest = "sha256:472206d4c501691d9e72cafca4362f2adbc610fecff3dfa42e5b345f9b7d05e5",  # 2018/10/25
    registry = "gcr.io",
    repository = "distroless/base",
    tag = "latest",
)

docker_pull(
    name = "alpine-base",
    digest = "sha256:bd327018b3effc802514b63cc90102bfcd92765f4486fc5abc28abf7eb9f1e4d",  # 2018/09/20
    registry = "gcr.io",
    repository = "k8s-prow/alpine",
    tag = "0.1",
)

docker_pull(
    name = "gcloud-base",
    digest = "sha256:1dbdee42a553dd6a652d64df1902015ba36ef12d6c16df568a59843e410e270b",  # 2018/10/25
    registry = "gcr.io",
    repository = "cloud-builders/gcloud",
    tag = "latest",
)

docker_pull(
    name = "git-base",
    digest = "sha256:01b0f83fe91b782ec7ddf1e742ab7cc9a2261894fd9ab0760ebfd39af2d6ab28",  # 2018/07/02
    registry = "gcr.io",
    repository = "k8s-prow/git",
    tag = "0.2",
)

docker_pull(
    name = "python",
    digest = "sha256:0888426cc407c5ce9f2d656d776757f8fdb31795e01f60df38a5bacb697a0db0",  # 2018/10/25
    registry = "index.docker.io",
    repository = "library/python",
    tag = "2",
)

git_repository(
    name = "io_bazel_rules_k8s",
    commit = "9d2f6e8e21f1b5e58e721fc29b806957d9931930",
    remote = "https://github.com/bazelbuild/rules_k8s.git",
)

git_repository(
    name = "io_kubernetes_build",
    commit = "4ce715fbe67d8fbed05ec2bb47a148e754100a4b",
    remote = "https://github.com/kubernetes/repo-infra.git",
)

git_repository(
    name = "build_bazel_rules_nodejs",
    remote = "https://github.com/bazelbuild/rules_nodejs.git",
    tag = "0.14.0",
)

load("@build_bazel_rules_nodejs//:defs.bzl", "node_repositories", "yarn_install")

node_repositories(package_json = ["//:package.json"])

yarn_install(
    name = "npm",
    package_json = "//:package.json",
    yarn_lock = "//:yarn.lock",
)

http_archive(
    name = "build_bazel_rules_typescript",
    strip_prefix = "rules_typescript-0.18.0",
    url = "https://github.com/bazelbuild/rules_typescript/archive/0.18.0.zip",
)

# Fetch our Bazel dependencies that aren't distributed on npm
load("@build_bazel_rules_typescript//:package.bzl", "rules_typescript_dependencies")

rules_typescript_dependencies()

# Setup TypeScript toolchain
load("@build_bazel_rules_typescript//:defs.bzl", "ts_setup_workspace")
load("//def:test_infra.bzl", "http_archive_with_pkg_path")

http_archive_with_pkg_path(
    name = "ruamel_yaml",
    build_file_content = """
py_library(
    name = "ruamel.yaml",
    srcs = glob(["*.py"]),
    visibility = ["//visibility:public"],
)
""",
    pkg_path = "ruamel/yaml",
    sha256 = "350496f6fdd8c2bb17a0fa3fd2ec98431280cf12d72dae498b19ac0119c2bbad",
    strip_prefix = "ruamel.yaml-0.15.9",
    url = "https://files.pythonhosted.org/packages/83/90/2eecde4bbd6a67805080091e83a29100c2f7d2afcaf926d75da5839f9283/ruamel.yaml-0.15.9.tar.gz",
)

# http_archives can be updated to newer version by doing the following:
#   1) replace the source url with the new source url.
#       -to find the newest version of a python module, search https://files.pythonhosted.org/ for the package.  Use the target url of the download link found at the bottom of the page.
#   2) replace the sha256 value with the sha256 sum of the updated package.
#       -pypi uses md5 sums not sha256 so run `md5sum xxxx.tar.gz` on the downloaded package and validate that it matches the md5sum on pypi
#       -once the package has been validated, determine the corresponding sha256 by running `sha256sum xxxx.tar.gz`.
#   3) ensure that the strip_prefix still prefixes the package directory contents to the level of the src code.

http_archive(
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
    urls = ["https://files.pythonhosted.org/packages/16/09/37b69de7c924d318e51ece1c4ceb679bf93be9d05973bb30c35babd596e2/requests-2.13.0.tar.gz"],
)

http_archive(
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
    urls = ["https://files.pythonhosted.org/packages/4a/85/db5a2df477072b2902b0eb892feb37d88ac635d36245a72a6a69b23b383a/PyYAML-3.12.tar.gz"],
)

http_archive(
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
    urls = ["https://files.pythonhosted.org/packages/4d/de/32d741db316d8fdb7680822dd37001ef7a448255de9699ab4bfcbdf4172b/MarkupSafe-1.0.tar.gz"],
)

http_archive(
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
    urls = ["https://files.pythonhosted.org/packages/71/59/d7423bd5e7ddaf3a1ce299ab4490e9044e8dfd195420fc83a24de9e60726/Jinja2-2.9.5.tar.gz"],
)

http_file(
    name = "jq_linux",
    executable = 1,
    sha256 = "c6b3a7d7d3e7b70c6f51b706a3b90bd01833846c54d32ca32f0027f00226ff6d",
    urls = ["https://github.com/stedolan/jq/releases/download/jq-1.5/jq-linux64"],
)

http_file(
    name = "jq_osx",
    executable = 1,
    sha256 = "386e92c982a56fe4851468d7a931dfca29560cee306a0e66c6a1bd4065d3dac5",
    urls = ["https://github.com/stedolan/jq/releases/download/jq-1.5/jq-osx-amd64"],
)

http_archive(
    name = "astroid_lib",
    build_file_content = """
py_library(
    name = "astroid_lib",
    srcs = glob(["**/*.py"]),
    deps = [
      "@six_lib//:six",
      "@wrapt//:wrapt",
      "@enum34//:enum34",
      "@lazy_object_proxy//:lazy_object_proxy",
      "@singledispatch_lib//:singledispatch_lib",
      "@backports//:backports",
    ],
    visibility = ["//visibility:public"],
    imports = ["astroid"],
)
""",
    sha256 = "492c2a2044adbf6a84a671b7522e9295ad2f6a7c781b899014308db25312dd35",
    strip_prefix = "astroid-1.5.3",
    urls = ["https://files.pythonhosted.org/packages/d7/b7/112288f75293d6f12b1e41bac1e822fd0f29b0f88e2c4378cdd295b9d838/astroid-1.5.3.tar.gz"],
)

http_archive(
    name = "isort",
    build_file_content = """
py_library(
    name = "isort",
    srcs = glob(["**/*.py"]),
    visibility = ["//visibility:public"],
)
""",
    sha256 = "79f46172d3a4e2e53e7016e663cc7a8b538bec525c36675fcfd2767df30b3983",
    strip_prefix = "isort-4.2.15/isort",
    urls = ["https://files.pythonhosted.org/packages/4d/d5/7c8657126a43bcd3b0173e880407f48be4ac91b4957b51303eab744824cf/isort-4.2.15.tar.gz"],
)

http_archive(
    name = "lazy_object_proxy",
    build_file_content = """
py_library(
    name = "lazy_object_proxy",
    srcs = glob(["**/*.py"]),
    visibility = ["//visibility:public"],
)
""",
    sha256 = "eb91be369f945f10d3a49f5f9be8b3d0b93a4c2be8f8a5b83b0571b8123e0a7a",
    strip_prefix = "lazy-object-proxy-1.3.1/src/lazy_object_proxy",
    urls = ["https://files.pythonhosted.org/packages/55/08/23c0753599bdec1aec273e322f277c4e875150325f565017f6280549f554/lazy-object-proxy-1.3.1.tar.gz"],
)

http_archive(
    name = "mccabe",
    build_file_content = """
py_library(
    name = "mccabe",
    srcs = glob(["**/*.py"]),
    visibility = ["//visibility:public"],
)
""",
    sha256 = "dd8d182285a0fe56bace7f45b5e7d1a6ebcbf524e8f3bd87eb0f125271b8831f",
    strip_prefix = "mccabe-0.6.1",
    urls = ["https://files.pythonhosted.org/packages/06/18/fa675aa501e11d6d6ca0ae73a101b2f3571a565e0f7d38e062eec18a91ee/mccabe-0.6.1.tar.gz"],
)

http_archive(
    name = "pylint",
    build_file_content = """
py_library(
    name = "pylint",
    srcs = glob(["**/*.py"]),
    deps = [
      "@astroid_lib//:astroid_lib",
      "@six_lib//:six",
      "@isort//:isort",
      "@mccabe//:mccabe",
      "@configparser_lib//:configparser_lib",
    ],
    visibility = ["//visibility:public"],
)
""",
    sha256 = "8b4a7ab6cf5062e40e2763c0b4a596020abada1d7304e369578b522e46a6264a",
    strip_prefix = "pylint-1.7.1/pylint",
    urls = [
        "https://files.pythonhosted.org/packages/cc/8c/d1da590769213fefedea4b345e90fce80f749c61ab9f9187b3fe19397b4b/pylint-1.7.1.tar.gz",
    ],
)

http_archive(
    name = "six_lib",
    build_file_content = """
py_library(
    name = "six",
    srcs = glob(["**/*.py"]),
    visibility = ["//visibility:public"],
)
""",
    sha256 = "105f8d68616f8248e24bf0e9372ef04d3cc10104f1980f54d57b2ce73a5ad56a",
    strip_prefix = "six-1.10.0",
    urls = ["https://files.pythonhosted.org/packages/b3/b2/238e2590826bfdd113244a40d9d3eb26918bd798fc187e2360a8367068db/six-1.10.0.tar.gz"],
)

http_archive(
    name = "wrapt",
    build_file_content = """
py_library(
    name = "wrapt",
    srcs = glob(["**/*.py"]),
    visibility = ["//visibility:public"],
)
""",
    sha256 = "42160c91b77f1bc64a955890038e02f2f72986c01d462d53cb6cb039b995cdd9",
    strip_prefix = "wrapt-1.10.10/src/wrapt",
    urls = ["https://files.pythonhosted.org/packages/a3/bb/525e9de0a220060394f4aa34fdf6200853581803d92714ae41fc3556e7d7/wrapt-1.10.10.tar.gz"],
)

http_archive(
    name = "enum34",
    build_file_content = """
py_library(
    name = "enum34",
    srcs = glob(["**/*.py"]),
    visibility = ["//visibility:public"],
)
""",
    sha256 = "8ad8c4783bf61ded74527bffb48ed9b54166685e4230386a9ed9b1279e2df5b1",
    strip_prefix = "enum34-1.1.6",
    urls = ["https://files.pythonhosted.org/packages/bf/3e/31d502c25302814a7c2f1d3959d2a3b3f78e509002ba91aea64993936876/enum34-1.1.6.tar.gz"],
)

http_archive(
    name = "singledispatch_lib",
    build_file_content = """
py_library(
    name = "singledispatch_lib",
    srcs = glob(["**/*.py"]),
    deps = [
      "@six_lib//:six",
    ],
    visibility = ["//visibility:public"],
)
""",
    sha256 = "5b06af87df13818d14f08a028e42f566640aef80805c3b50c5056b086e3c2b9c",
    strip_prefix = "singledispatch-3.4.0.3",
    urls = ["https://files.pythonhosted.org/packages/d9/e9/513ad8dc17210db12cb14f2d4d190d618fb87dd38814203ea71c87ba5b68/singledispatch-3.4.0.3.tar.gz"],
)

http_archive(
    name = "backports",
    build_file_content = """
py_library(
    name = "backports",
    srcs = glob(["**/*.py"]),
    visibility = ["//visibility:public"],
)
""",
    sha256 = "31f235852f88edc1558d428d890663c49eb4514ffec9f3650e7f3c9e4a12e36f",
    strip_prefix = "backports.functools_lru_cache-1.4/backports",
    urls = ["https://files.pythonhosted.org/packages/4e/91/0e93d9455254b7b630fb3ebe30cc57cab518660c5fad6a08aac7908a4431/backports.functools_lru_cache-1.4.tar.gz"],
)

http_archive(
    name = "configparser_lib",
    build_file_content = """
py_library(
    name = "configparser_lib",
    srcs = glob(["**/*.py"]),
    visibility = ["//visibility:public"],
    imports = ["backports"],
)
""",
    sha256 = "5308b47021bc2340965c371f0f058cc6971a04502638d4244225c49d80db273a",
    strip_prefix = "configparser-3.5.0/src",
    urls = ["https://files.pythonhosted.org/packages/7c/69/c2ce7e91c89dc073eb1aa74c0621c3eefbffe8216b3f9af9d3885265c01c/configparser-3.5.0.tar.gz"],
)

# find the most recent version of influxdb at https://pypi.python.org/pypi/influxdb/
http_archive(
    name = "influxdb",
    build_file_content = """
py_library(
    name = "influxdb",
    srcs = glob(["**/*.py"]),
    visibility = ["//visibility:public"],
)
""",
    sha256 = "e3790474fa6d3e39043471a2a79b3309e9a47c63c0803a8810241bc8ce056b18",
    strip_prefix = "influxdb-4.1.1/influxdb",
    urls = ["https://files.pythonhosted.org/packages/e1/af/94faea244de2a73b7a0087637660db2d638edaae58f22d3f0d0d219ad8b7/influxdb-4.1.1.tar.gz"],
)

# find the most recent version at https://pypi.python.org/pypi/pytz
http_archive(
    name = "pytz",
    build_file_content = """
py_library(
    name = "pytz",
    srcs = glob(["**/*.py"]),
    visibility = ["//visibility:public"],
)
""",
    sha256 = "f5c056e8f62d45ba8215e5cb8f50dfccb198b4b9fbea8500674f3443e4689589",
    strip_prefix = "pytz-2017.2/pytz",
    urls = ["https://files.pythonhosted.org/packages/a4/09/c47e57fc9c7062b4e83b075d418800d322caa87ec0ac21e6308bd3a2d519/pytz-2017.2.zip"],
)

# find the most recent version at https://pypi.python.org/pypi/python-dateutil
http_archive(
    name = "dateutil",
    build_file_content = """
py_library(
    name = "dateutil",
    srcs = glob(["**/*.py"]),
    deps = [
        "@six_lib//:six",
    ],
    visibility = ["//visibility:public"],
)
""",
    sha256 = "891c38b2a02f5bb1be3e4793866c8df49c7d19baabf9c1bad62547e0b4866aca",
    strip_prefix = "python-dateutil-2.6.1/dateutil",
    urls = ["https://files.pythonhosted.org/packages/54/bb/f1db86504f7a49e1d9b9301531181b00a1c7325dc85a29160ee3eaa73a54/python-dateutil-2.6.1.tar.gz"],
)

# TODO(fejta): get this to work
git_repository(
    name = "io_bazel_rules_appengine",
    commit = "14d860985c2a764fdb6a6072d5450d8360c4ce5b",
    remote = "https://github.com/bazelbuild/rules_appengine.git",
    #tag = "0.0.5", # Latest at https://github.com/bazelbuild/rules_appengine/releases.
)

load("@io_bazel_rules_appengine//appengine:py_appengine.bzl", "py_appengine_repositories")

py_appengine_repositories()

git_repository(
    name = "io_bazel_rules_python",
    commit = "f414af5ed85e451908b3fb873211e8f2939ea4e8",
    remote = "https://github.com/bazelbuild/rules_python.git",
)

# Only needed for PIP support:
load("@io_bazel_rules_python//python:pip.bzl", "pip_import")

pip_import(
    name = "kettle_deps",
    requirements = "//kettle:requirements.txt",
)

load("@kettle_deps//:requirements.bzl", "pip_install")

pip_install()
