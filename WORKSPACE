workspace(name = "io_k8s_test_infra")

load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive", "http_file")
load("@bazel_tools//tools/build_defs/repo:git.bzl", "git_repository")

http_archive(
    name = "bazel_toolchains",
    sha256 = "8d43844d1d4447be2a108834771d617a1ad2a107f1680190bfe44925e7bf530e",
    strip_prefix = "bazel-toolchains-4c003ad45e8a2d829ffc40e3aecfb6b8577a9406",
    urls = [
        "https://mirror.bazel.build/github.com/bazelbuild/bazel-toolchains/archive/4c003ad45e8a2d829ffc40e3aecfb6b8577a9406.tar.gz",
        "https://github.com/bazelbuild/bazel-toolchains/archive/4c003ad45e8a2d829ffc40e3aecfb6b8577a9406.tar.gz",
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

versions.check(minimum_bazel_version = "0.23.0")

# TODO(fejta): delete the com_google_protobuf rule once rules_go >= 0.18.4
# This silences the shallow_since message (and increases cache hit rate)
git_repository(
    name = "com_google_protobuf",
    remote = "https://github.com/protocolbuffers/protobuf",
    commit = "582743bf40c5d3639a70f98f183914a2c0cd0680",  # v3.7.0, as of 2019-03-03
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

load("@bazel_gazelle//:deps.bzl", "gazelle_dependencies", "go_repository")

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

git_repository(
    name = "build_bazel_rules_nodejs",
    commit = "0eb4a19507211ab3863f4d82e9412a33f759abcd",
    remote = "https://github.com/bazelbuild/rules_nodejs.git",
    shallow_since = "1548802468 -0800",
    #tag = "0.16.6",
)

load("@build_bazel_rules_nodejs//:defs.bzl", "node_repositories", "yarn_install")

node_repositories(package_json = ["//:package.json"])

yarn_install(
    name = "npm",
    package_json = "//:package.json",
    quiet = True,
    yarn_lock = "//:yarn.lock",
)

http_archive(
    name = "build_bazel_rules_typescript",
    sha256 = "136ba6be39b4ff934cc0f41f043912305e98cb62254d9e6af467e247daafcd34",
    strip_prefix = "rules_typescript-0.22.0",
    url = "https://github.com/bazelbuild/rules_typescript/archive/0.22.0.zip",
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
    commit = "fdbce051adecbb369b15260046f4f23684369efc",
    remote = "https://github.com/bazelbuild/rules_appengine.git",
    shallow_since = "1552415147 -0400",
    #tag = "0.0.8+but-this-isn't-new-enough", # Latest at https://github.com/bazelbuild/rules_appengine/releases.
)

load("@io_bazel_rules_appengine//appengine:py_appengine.bzl", "py_appengine_repositories")

py_appengine_repositories()

git_repository(
    name = "io_bazel_rules_python",
    commit = "cc4cbf2f042695f4d1d4198c22459b3dbe7f8e43",
    remote = "https://github.com/bazelbuild/rules_python.git",
    shallow_since = "1546820050 -0500",
)

# Only needed for PIP support:
load("@io_bazel_rules_python//python:pip.bzl", "pip_import")

pip_import(
    name = "kettle_deps",
    requirements = "//kettle:requirements.txt",
)

load(
    "@kettle_deps//:requirements.bzl",
    kettle_pip_install = "pip_install",
)

kettle_pip_install()

pip_import(
    name = "pylint_deps",
    requirements = "//hack:requirements.txt",
)

load(
    "@pylint_deps//:requirements.bzl",
    pylint_pip_install = "pip_install",
)

pylint_pip_install()

go_repository(
    name = "com_github_client9_misspell",
    commit = "9ce5d979ffdaca6385988d7ad1079a33ec942d20",
    importpath = "github.com/client9/misspell",
)

## Repos generated by hack/update-deps.sh below

go_repository(
    name = "co_honnef_go_tools",
    importpath = "honnef.co/go/tools",
    sum = "h1:/8zB6iBfHCl1qAnEAWwGPNrUvapuy6CPla1VM0k8hQw=",
    version = "v0.0.0-20190106161140-3f1c8253044a",
)

go_repository(
    name = "com_github_andygrunwald_go_gerrit",
    importpath = "github.com/andygrunwald/go-gerrit",
    sum = "h1:uUuUZipfD5nPl2L/i0I3N4iRKJcoO2CPjktaH/kP9gQ=",
    version = "v0.0.0-20190120104749-174420ebee6c",
)

go_repository(
    name = "com_github_aws_aws_k8s_tester",
    importpath = "github.com/aws/aws-k8s-tester",
    sum = "h1:B7prfUXk9GHnMc/a7NrEY29IJRgI4/2cCbHDqQJGgLs=",
    version = "v0.0.0-20190114231546-b411acf57dfe",
)

go_repository(
    name = "com_github_aws_aws_sdk_go",
    importpath = "github.com/aws/aws-sdk-go",
    sum = "h1:POeH34ZME++pr7GBGh+ZO6Y5kOwSMQpqp5BGUgooJ6k=",
    version = "v1.16.36",
)

go_repository(
    name = "com_github_azure_azure_pipeline_go",
    importpath = "github.com/Azure/azure-pipeline-go",
    sum = "h1:u7JFb9fFTE6Y/j8ae2VK33ePrRqJqoCM/IWkQdAZ+rg=",
    version = "v0.1.9",
)

go_repository(
    name = "com_github_azure_azure_sdk_for_go",
    importpath = "github.com/Azure/azure-sdk-for-go",
    sum = "h1:43w4ZSx8u8wh5tgs1P+hO5Dgjh5bhP0qAlEnCdAuMQA=",
    version = "v21.1.0+incompatible",
)

go_repository(
    name = "com_github_azure_azure_storage_blob_go",
    importpath = "github.com/Azure/azure-storage-blob-go",
    sum = "h1:QjGHsWFbJyl312t0BtgkmZy2TTYA++FF0UakGbr3ZhQ=",
    version = "v0.0.0-20190123011202-457680cc0804",
)

go_repository(
    name = "com_github_azure_go_autorest",
    importpath = "github.com/Azure/go-autorest",
    sum = "h1:vdxx6wM1rVkKt/3niByPVjguoLWkWImOcJNvEykgBzY=",
    version = "v10.15.5+incompatible",
)

go_repository(
    name = "com_github_bazelbuild_buildtools",
    importpath = "github.com/bazelbuild/buildtools",
    sum = "h1:WHyHxJd9ZTyyXqlEuheY93v4cPJLBy0SIEshPCcK0xQ=",
    version = "v0.0.0-20190404153937-93253d6efaa9",
)

go_repository(
    name = "com_github_beorn7_perks",
    importpath = "github.com/beorn7/perks",
    sum = "h1:xJ4a3vCFaGF/jqvzLMYoU8P317H5OQ+Via4RmuPwCS0=",
    version = "v0.0.0-20180321164747-3a771d992973",
)

go_repository(
    name = "com_github_bgentry_speakeasy",
    importpath = "github.com/bgentry/speakeasy",
    sum = "h1:ByYyxL9InA1OWqxJqqp2A5pYHUrCiAL6K3J+LKSsQkY=",
    version = "v0.1.0",
)

go_repository(
    name = "com_github_blang_semver",
    importpath = "github.com/blang/semver",
    sum = "h1:cQNTCjp13qL8KC3Nbxr/y2Bqb63oX6wdnnjpJbkM4JQ=",
    version = "v3.5.1+incompatible",
)

go_repository(
    name = "com_github_burntsushi_toml",
    importpath = "github.com/BurntSushi/toml",
    sum = "h1:WXkYYl6Yr3qBf1K79EBnL4mak0OimBfB0XUf9Vl28OQ=",
    version = "v0.3.1",
)

go_repository(
    name = "com_github_bwmarrin_snowflake",
    importpath = "github.com/bwmarrin/snowflake",
    sum = "h1:dRbqXFjM10uA3wdrVZ8Kh19uhciRMOroUYJ7qAqDLhY=",
    version = "v0.0.0",
)

go_repository(
    name = "com_github_client9_misspell",
    importpath = "github.com/client9/misspell",
    tag = "v0.3.4",
)

go_repository(
    name = "com_github_coreos_go_semver",
    importpath = "github.com/coreos/go-semver",
    sum = "h1:3Jm3tLmsgAYcjC+4Up7hJrFBPr+n7rAqYeSw/SZazuY=",
    version = "v0.2.0",
)

go_repository(
    name = "com_github_coreos_go_systemd",
    importpath = "github.com/coreos/go-systemd",
    sum = "h1:u9SHYsPQNyt5tgDm3YN7+9dYrpK96E5wFilTFWIDZOM=",
    version = "v0.0.0-20180511133405-39ca1b05acc7",
)

go_repository(
    name = "com_github_coreos_pkg",
    importpath = "github.com/coreos/pkg",
    sum = "h1:CAKfRE2YtTUIjjh1bkBtyYFaUT/WmOqsJjgtihT0vMI=",
    version = "v0.0.0-20160727233714-3ac0863d7acf",
)

go_repository(
    name = "com_github_davecgh_go_spew",
    importpath = "github.com/davecgh/go-spew",
    sum = "h1:vj9j/u1bqnvCEfJOwUhtlOARqs3+rkHYY13jYWTU97c=",
    version = "v1.1.1",
)

go_repository(
    name = "com_github_deckarep_golang_set",
    importpath = "github.com/deckarep/golang-set",
    sum = "h1:njG8LmGD6JCWJu4bwIKmkOHvch70UOEIqczl5vp7Gok=",
    version = "v0.0.0-20171013212420-1d4478f51bed",
)

go_repository(
    name = "com_github_denisenkom_go_mssqldb",
    importpath = "github.com/denisenkom/go-mssqldb",
    sum = "h1:M0bjbJ5PZPl4iKkt0FSvhfSCJI9NisDDda29jXN9i0c=",
    version = "v0.0.0-20190111225525-2fea367d496d",
)

go_repository(
    name = "com_github_dgrijalva_jwt_go",
    importpath = "github.com/dgrijalva/jwt-go",
    sum = "h1:7qlOGliEKZXTDg6OTjfoBKDXWrumCAMpl/TFQ4/5kLM=",
    version = "v3.2.0+incompatible",
)

go_repository(
    name = "com_github_djherbis_atime",
    importpath = "github.com/djherbis/atime",
    sum = "h1:ySLvBAM0EvOGaX7TI4dAM5lWj+RdJUCKtGSEHN8SGBg=",
    version = "v1.0.0",
)

go_repository(
    name = "com_github_docker_distribution",
    importpath = "github.com/docker/distribution",
    sum = "h1:FKH02LHYqSmeWd3GBh0KIkM8JBpw3RrShgtcWShdWJg=",
    version = "v0.0.0-20170726174610-edc3ab29cdff",
)

go_repository(
    name = "com_github_docker_docker",
    importpath = "github.com/docker/docker",
    sum = "h1:aiKCZxnxoraZKXbM2zJj0ZI1n8eamkdK7zNxP7jlbII=",
    version = "v0.0.0-20171206114025-5e5fadb3c020",
)

go_repository(
    name = "com_github_docker_go_connections",
    importpath = "github.com/docker/go-connections",
    sum = "h1:3lOnM9cSzgGwx8VfK/NGOW5fLQ0GjIlCkaktF+n1M6o=",
    version = "v0.3.0",
)

go_repository(
    name = "com_github_docker_go_units",
    importpath = "github.com/docker/go-units",
    sum = "h1:Xk8S3Xj5sLGlG5g67hJmYMmUgXv5N4PhkjJHHqrwnTk=",
    version = "v0.3.3",
)

go_repository(
    name = "com_github_dustin_go_humanize",
    importpath = "github.com/dustin/go-humanize",
    sum = "h1:VSnTsYCnlFHaM2/igO1h6X3HA71jcobQuxemgkq4zYo=",
    version = "v1.0.0",
)

go_repository(
    name = "com_github_erikstmartin_go_testdb",
    importpath = "github.com/erikstmartin/go-testdb",
    sum = "h1:Yzb9+7DPaBjB8zlTR87/ElzFsnQfuHnVUVqpZZIcV5Y=",
    version = "v0.0.0-20160219214506-8d10e4a1bae5",
)

go_repository(
    name = "com_github_evanphx_json_patch",
    importpath = "github.com/evanphx/json-patch",
    sum = "h1:K1MDoo4AZ4wU0GIU/fPmtZg7VpzLjCxu+UwBD1FvwOc=",
    version = "v4.1.0+incompatible",
)

go_repository(
    name = "com_github_fatih_color",
    importpath = "github.com/fatih/color",
    sum = "h1:DkWD4oS2D8LGGgTQ6IvwJJXSL5Vp2ffcQg58nFV38Ys=",
    version = "v1.7.0",
)

go_repository(
    name = "com_github_fsnotify_fsnotify",
    importpath = "github.com/fsnotify/fsnotify",
    sum = "h1:IXs+QLmnXW2CcXuY+8Mzv/fWEsPGWxqefPtCP5CnV9I=",
    version = "v1.4.7",
)

go_repository(
    name = "com_github_fsouza_fake_gcs_server",
    importpath = "github.com/fsouza/fake-gcs-server",
    sum = "h1:3iml5UHzQtk3cpnYfqW16Ia+1xSuu9tc4BElZu5470M=",
    version = "v0.0.0-20180612165233-e85be23bdaa8",
)

go_repository(
    name = "com_github_ghodss_yaml",
    importpath = "github.com/ghodss/yaml",
    sum = "h1:wQHKEahhL6wmXdzwWG11gIVCkOv05bNOh+Rxn0yngAk=",
    version = "v1.0.0",
)

go_repository(
    name = "com_github_go_openapi_jsonpointer",
    importpath = "github.com/go-openapi/jsonpointer",
    sum = "h1:UmnefiS/Yrdfl15NXUA9T51lyQf72tCvWHfOiRLd1+g=",
    version = "v0.0.0-20170102174223-779f45308c19",
)

go_repository(
    name = "com_github_go_openapi_jsonreference",
    importpath = "github.com/go-openapi/jsonreference",
    sum = "h1:gbNUNGpVJLxaXBxI7iCHZdg3PwgLOJ9lPQGVINRrC9E=",
    version = "v0.0.0-20161105162150-36d33bfe519e",
)

go_repository(
    name = "com_github_go_openapi_spec",
    importpath = "github.com/go-openapi/spec",
    sum = "h1:/4LLqbrF/Zoeyra76KP3b3I/D3II/lSSFVFDjiEaQKM=",
    version = "v0.0.0-20171219195406-fa03337d7da5",
)

go_repository(
    name = "com_github_go_openapi_swag",
    importpath = "github.com/go-openapi/swag",
    sum = "h1:r38dWW65/T34YOCwzGTAoDxP16l+OpDJl/n07P+pvUo=",
    version = "v0.0.0-20171111214437-cf0bdb963811",
)

go_repository(
    name = "com_github_go_sql_driver_mysql",
    importpath = "github.com/go-sql-driver/mysql",
    sum = "h1:cRYCw9putX9pmpNdGkE/rWrY6gNGqZvdgGT1RPl6K18=",
    version = "v0.0.0-20160411075031-7ebe0a500653",
)

go_repository(
    name = "com_github_go_yaml_yaml",
    importpath = "github.com/go-yaml/yaml",
    sum = "h1:RYi2hDdss1u4YE7GwixGzWwVo47T8UQwnTLB6vQiq+o=",
    version = "v2.1.0+incompatible",
)

go_repository(
    name = "com_github_gogo_protobuf",
    importpath = "github.com/gogo/protobuf",
    sum = "h1:/s5zKNz0uPFCZ5hddgPdo2TK2TVrUNMn0OOX8/aZMTE=",
    version = "v1.2.1",
)

go_repository(
    name = "com_github_golang_glog",
    importpath = "github.com/golang/glog",
    sum = "h1:VKtxabqXZkF25pY9ekfRL6a582T4P37/31XEstQ5p58=",
    version = "v0.0.0-20160126235308-23def4e6c14b",
)

go_repository(
    name = "com_github_golang_groupcache",
    importpath = "github.com/golang/groupcache",
    sum = "h1:u4bArs140e9+AfE52mFHOXVFnOSBJBRlzTHrOPLOIhE=",
    version = "v0.0.0-20180513044358-24b0969c4cb7",
)

go_repository(
    name = "com_github_golang_lint",
    commit = "06c8688daad7",
    importpath = "github.com/golang/lint",
)

go_repository(
    name = "com_github_golang_mock",
    importpath = "github.com/golang/mock",
    sum = "h1:28o5sBqPkBsMGnC6b4MvE2TzSr5/AT4c/1fLqVGIwlk=",
    version = "v1.2.0",
)

go_repository(
    name = "com_github_golang_protobuf",
    importpath = "github.com/golang/protobuf",
    sum = "h1:YF8+flBXS5eO826T4nzqPrxfhQThhXl0YzfuUPu4SBg=",
    version = "v1.3.1",
)

go_repository(
    name = "com_github_google_btree",
    importpath = "github.com/google/btree",
    sum = "h1:964Od4U6p2jUkFxvCydnIczKteheJEzHRToSGK3Bnlw=",
    version = "v0.0.0-20180813153112-4030bb1f1f0c",
)

go_repository(
    name = "com_github_google_go_cmp",
    importpath = "github.com/google/go-cmp",
    sum = "h1:+dTQ8DZQJz0Mb/HjFlkptS1FeQ4cWSnN941F8aEG4SQ=",
    version = "v0.2.0",
)

go_repository(
    name = "com_github_google_go_github",
    importpath = "github.com/google/go-github",
    sum = "h1:N0LgJ1j65A7kfXrZnUDaYCs/Sf4rEjNlfyDHW9dolSY=",
    version = "v17.0.0+incompatible",
)

go_repository(
    name = "com_github_google_go_querystring",
    importpath = "github.com/google/go-querystring",
    sum = "h1:Xkwi/a1rcvNg1PPYe5vI8GbeBY/jrVuDX5ASuANWTrk=",
    version = "v1.0.0",
)

go_repository(
    name = "com_github_google_gofuzz",
    importpath = "github.com/google/gofuzz",
    sum = "h1:+RRA9JqSOZFfKrOeqr2z77+8R2RKyh8PG66dcu1V0ck=",
    version = "v0.0.0-20170612174753-24818f796faf",
)

go_repository(
    name = "com_github_google_martian",
    importpath = "github.com/google/martian",
    sum = "h1:/CP5g8u/VJHijgedC/Legn3BAbAaWPgecwXBIDzw5no=",
    version = "v2.1.0+incompatible",
)

go_repository(
    name = "com_github_google_uuid",
    importpath = "github.com/google/uuid",
    sum = "h1:b4Gk+7WdP/d3HZH8EJsZpvV7EtDOgaZLtnaNGIu1adA=",
    version = "v1.0.0",
)

go_repository(
    name = "com_github_googleapis_gax_go",
    importpath = "github.com/googleapis/gax-go",
    sum = "h1:j0GKcs05QVmm7yesiZq2+9cxHkNK9YM6zKx4D2qucQU=",
    version = "v2.0.0+incompatible",
)

go_repository(
    name = "com_github_googleapis_gnostic",
    importpath = "github.com/googleapis/gnostic",
    sum = "h1:rVsPeBmXbYv4If/cumu1AzZPwV58q433hvONV1UEZoI=",
    version = "v0.1.0",
)

go_repository(
    name = "com_github_gophercloud_gophercloud",
    importpath = "github.com/gophercloud/gophercloud",
    sum = "h1:rT82/6k75XPwKx/5VJZSODZMF0/YgGhOJlGzchTdL7I=",
    version = "v0.0.0-20181215224939-bdd8b1ecd793",
)

go_repository(
    name = "com_github_gorilla_context",
    importpath = "github.com/gorilla/context",
    sum = "h1:AWwleXJkX/nhcU9bZSnZoi3h/qGYqQAGhq6zZe/aQW8=",
    version = "v1.1.1",
)

go_repository(
    name = "com_github_gorilla_mux",
    importpath = "github.com/gorilla/mux",
    sum = "h1:Pgr17XVTNXAk3q/r4CpKzC5xBM/qW1uVLV+IhRZpIIk=",
    version = "v1.6.2",
)

go_repository(
    name = "com_github_gorilla_securecookie",
    importpath = "github.com/gorilla/securecookie",
    sum = "h1:miw7JPhV+b/lAHSXz4qd/nN9jRiAFV5FwjeKyCS8BvQ=",
    version = "v1.1.1",
)

go_repository(
    name = "com_github_gorilla_sessions",
    importpath = "github.com/gorilla/sessions",
    sum = "h1:uXoZdcdA5XdXF3QzuSlheVRUvjl+1rKY7zBXL68L9RU=",
    version = "v1.1.3",
)

go_repository(
    name = "com_github_gorilla_websocket",
    importpath = "github.com/gorilla/websocket",
    sum = "h1:Lh2aW+HnU2Nbe1gqD9SOJLJxW1jBMmQOktN2acDyJk8=",
    version = "v0.0.0-20170926233335-4201258b820c",
)

go_repository(
    name = "com_github_gregjones_httpcache",
    importpath = "github.com/gregjones/httpcache",
    sum = "h1:pdN6V1QBWetyv/0+wjACpqVH+eVULgEjkurDLq3goeM=",
    version = "v0.0.0-20180305231024-9cad4c3443a7",
)

go_repository(
    name = "com_github_grpc_ecosystem_go_grpc_middleware",
    importpath = "github.com/grpc-ecosystem/go-grpc-middleware",
    sum = "h1:Iju5GlWwrvL6UBg4zJJt3btmonfrMlCDdsejg4CZE7c=",
    version = "v1.0.0",
)

go_repository(
    name = "com_github_grpc_ecosystem_go_grpc_prometheus",
    importpath = "github.com/grpc-ecosystem/go-grpc-prometheus",
    sum = "h1:Ovs26xHkKqVztRpIrF/92BcuyuQ/YW4NSIpoGtfXNho=",
    version = "v1.2.0",
)

go_repository(
    name = "com_github_grpc_ecosystem_grpc_gateway",
    importpath = "github.com/grpc-ecosystem/grpc-gateway",
    sum = "h1:pX7cnDwSSmG0dR9yNjCQSSpmsJOqFdT7SzVp5Yl9uVw=",
    version = "v1.4.1",
)

go_repository(
    name = "com_github_hashicorp_errwrap",
    importpath = "github.com/hashicorp/errwrap",
    sum = "h1:prjrVgOk2Yg6w+PflHoszQNLTUh4kaByUcEWM/9uin4=",
    version = "v0.0.0-20141028054710-7554cd9344ce",
)

go_repository(
    name = "com_github_hashicorp_go_multierror",
    importpath = "github.com/hashicorp/go-multierror",
    sum = "h1:em+tTnzgU7N22woTBMcSJAOW7tRHAkK597W+MD/CpK8=",
    version = "v0.0.0-20171204182908-b7773ae21874",
)

go_repository(
    name = "com_github_hashicorp_golang_lru",
    importpath = "github.com/hashicorp/golang-lru",
    sum = "h1:0hERBMJE1eitiLkihrMvRVBYAkpHzc/J3QdDN+dAcgU=",
    version = "v0.5.1",
)

go_repository(
    name = "com_github_hpcloud_tail",
    importpath = "github.com/hpcloud/tail",
    sum = "h1:nfCOvKYfkgYP8hkirhJocXT2+zOD8yUNjXaWfTlyFKI=",
    version = "v1.0.0",
)

go_repository(
    name = "com_github_imdario_mergo",
    importpath = "github.com/imdario/mergo",
    sum = "h1:geJOJJZwkYI1yqxWrAMcgrwDvy4P1XyNNgIyN9d6UXc=",
    version = "v0.0.0-20180119215619-163f41321a19",
)

go_repository(
    name = "com_github_inconshreveable_mousetrap",
    importpath = "github.com/inconshreveable/mousetrap",
    sum = "h1:Z8tu5sraLXCXIcARxBp/8cbvlwVa7Z1NHg9XEKhtSvM=",
    version = "v1.0.0",
)

go_repository(
    name = "com_github_influxdata_influxdb",
    importpath = "github.com/influxdata/influxdb",
    sum = "h1:AciJ2ei/llFRundm7CtqwF6B2aOds1A7QG3sMW8QiaQ=",
    version = "v0.0.0-20161215172503-049f9b42e9a5",
)

go_repository(
    name = "com_github_jinzhu_gorm",
    importpath = "github.com/jinzhu/gorm",
    sum = "h1:0D5F4qAGJbRqzyCIHswU2fCwB1XGTDkBwBn9qncQYYs=",
    version = "v0.0.0-20170316141641-572d0a0ab1eb",
)

go_repository(
    name = "com_github_jinzhu_inflection",
    importpath = "github.com/jinzhu/inflection",
    sum = "h1:+boKnC0mZ4voXeGF5Qy5Hyx4A17u59wSv42zjTZdxTQ=",
    version = "v0.0.0-20151009084129-3272df6c21d0",
)

go_repository(
    name = "com_github_jinzhu_now",
    importpath = "github.com/jinzhu/now",
    sum = "h1:xvj06l8iSwiWpYgm8MbPp+naBg+pwfqmdXabzqPCn/8=",
    version = "v0.0.0-20181116074157-8ec929ed50c3",
)

go_repository(
    name = "com_github_jmespath_go_jmespath",
    importpath = "github.com/jmespath/go-jmespath",
    sum = "h1:pmfjZENx5imkbgOkpRUYLnmbU7UEFbjtDA2hxJ1ichM=",
    version = "v0.0.0-20180206201540-c2b33e8439af",
)

go_repository(
    name = "com_github_jonboulle_clockwork",
    importpath = "github.com/jonboulle/clockwork",
    sum = "h1:VKV+ZcuP6l3yW9doeqz6ziZGgcynBVQO+obU0+0hcPo=",
    version = "v0.1.0",
)

go_repository(
    name = "com_github_joshdk_go_junit",
    commit = "bf76511d0869",
    importpath = "github.com/joshdk/go-junit",
)

go_repository(
    name = "com_github_json_iterator_go",
    importpath = "github.com/json-iterator/go",
    sum = "h1:MrUvLMLTMxbqFJ9kzlvat/rYZqZnW3u4wkLzWTaFwKs=",
    version = "v1.1.6",
)

go_repository(
    name = "com_github_kisielk_gotool",
    importpath = "github.com/kisielk/gotool",
    sum = "h1:AV2c/EiW3KqPNT9ZKl07ehoAGi4C5/01Cfbblndcapg=",
    version = "v1.0.0",
)

go_repository(
    name = "com_github_knative_build",
    importpath = "github.com/knative/build",
    sum = "h1:3KvuoUpgXYkK3WKaSBVN/0FSwRaukf212t2bQkMSWjY=",
    version = "v0.3.1-0.20190330033454-38ace00371c7",
)

go_repository(
    name = "com_github_knative_pkg",
    importpath = "github.com/knative/pkg",
    sum = "h1:WE+sMoCzGTt6OhWGWkXwtuzmfqofaqfyx0i9qte+NLQ=",
    version = "v0.0.0-20190330034653-916205998db9",
)

go_repository(
    name = "com_github_konsorten_go_windows_terminal_sequences",
    importpath = "github.com/konsorten/go-windows-terminal-sequences",
    sum = "h1:DB17ag19krx9CFsz4o3enTrPXyIXCl+2iCXH/aMAp9s=",
    version = "v1.0.2",
)

go_repository(
    name = "com_github_kr_pty",
    importpath = "github.com/kr/pty",
    sum = "h1:VkoXIwSboBpnk99O/KFauAEILuNHv5DVFKZMBN/gUgw=",
    version = "v1.1.1",
)

go_repository(
    name = "com_github_lib_pq",
    importpath = "github.com/lib/pq",
    sum = "h1:X5PMW56eZitiTeO7tKzZxFCSpbFZJtkMMooicw2us9A=",
    version = "v1.0.0",
)

go_repository(
    name = "com_github_mailru_easyjson",
    importpath = "github.com/mailru/easyjson",
    sum = "h1:bM4HYnlVXPgUKmzl7o3drEaVfOk+sTBiADAQOWjU+8I=",
    version = "v0.0.0-20171120080333-32fa128f234d",
)

go_repository(
    name = "com_github_mattbaird_jsonpatch",
    importpath = "github.com/mattbaird/jsonpatch",
    sum = "h1:+J2gw7Bw77w/fbK7wnNJJDKmw1IbWft2Ul5BzrG1Qm8=",
    version = "v0.0.0-20171005235357-81af80346b1a",
)

go_repository(
    name = "com_github_mattn_go_colorable",
    importpath = "github.com/mattn/go-colorable",
    sum = "h1:UVL0vNpWh04HeJXV0KLcaT7r06gOH2l4OW6ddYRUIY4=",
    version = "v0.0.9",
)

go_repository(
    name = "com_github_mattn_go_isatty",
    importpath = "github.com/mattn/go-isatty",
    sum = "h1:bnP0vzxcAdeI1zdubAl5PjU6zsERjGZb7raWodagDYs=",
    version = "v0.0.4",
)

go_repository(
    name = "com_github_mattn_go_runewidth",
    importpath = "github.com/mattn/go-runewidth",
    sum = "h1:UnlwIPBGaTZfPQ6T1IGzPI0EkYAQmT9fAEJ/poFC63o=",
    version = "v0.0.2",
)

go_repository(
    name = "com_github_mattn_go_sqlite3",
    importpath = "github.com/mattn/go-sqlite3",
    sum = "h1:LeyN6zuPxHQrv7ZGHbo2Ymtj+BfpbG02+5iAgKds/zU=",
    version = "v0.0.0-20160514122348-38ee283dabf1",
)

go_repository(
    name = "com_github_mattn_go_zglob",
    importpath = "github.com/mattn/go-zglob",
    sum = "h1:xsEx/XUoVlI6yXjqBK062zYhRTZltCNmYPx6v+8DNaY=",
    version = "v0.0.1",
)

go_repository(
    name = "com_github_matttproud_golang_protobuf_extensions",
    importpath = "github.com/matttproud/golang_protobuf_extensions",
    sum = "h1:4hp9jkHxhMHkqkrB3Ix0jegS5sx/RkqARlsWZ6pIwiU=",
    version = "v1.0.1",
)

go_repository(
    name = "com_github_microsoft_go_winio",
    importpath = "github.com/Microsoft/go-winio",
    sum = "h1:xAfWHN1IrQ0NJ9TBC0KBZoqLjzDTr1ML+4MywiUOryc=",
    version = "v0.4.12",
)

go_repository(
    name = "com_github_mitchellh_ioprogress",
    importpath = "github.com/mitchellh/ioprogress",
    sum = "h1:Qa6dnn8DlasdXRnacluu8HzPts0S1I9zvvUPDbBnXFI=",
    version = "v0.0.0-20180201004757-6a23b12fa88e",
)

go_repository(
    name = "com_github_modern_go_concurrent",
    importpath = "github.com/modern-go/concurrent",
    sum = "h1:TRLaZ9cD/w8PVh93nsPXa1VrQ6jlwL5oN8l14QlcNfg=",
    version = "v0.0.0-20180306012644-bacd9c7ef1dd",
)

go_repository(
    name = "com_github_modern_go_reflect2",
    importpath = "github.com/modern-go/reflect2",
    sum = "h1:9f412s+6RmYXLWZSEzVVgPGK7C2PphHj5RJrvfx9AWI=",
    version = "v1.0.1",
)

go_repository(
    name = "com_github_nytimes_gziphandler",
    importpath = "github.com/NYTimes/gziphandler",
    sum = "h1:YmY+w0aFpjdvy2zf8dxbSeLfFeE9PqCh21knryKHtok=",
    version = "v0.0.0-20160419202541-63027b26b87e",
)

go_repository(
    name = "com_github_olekukonko_tablewriter",
    importpath = "github.com/olekukonko/tablewriter",
    sum = "h1:58+kh9C6jJVXYjt8IE48G2eWl6BjwU5Gj0gqY84fy78=",
    version = "v0.0.0-20170122224234-a0225b3f23b5",
)

go_repository(
    name = "com_github_onsi_ginkgo",
    importpath = "github.com/onsi/ginkgo",
    sum = "h1:WSHQ+IS43OoUrWtD1/bbclrwK8TTH5hzp+umCiuxHgs=",
    version = "v1.7.0",
)

go_repository(
    name = "com_github_onsi_gomega",
    importpath = "github.com/onsi/gomega",
    sum = "h1:RE1xgDvH7imwFD45h+u2SgIfERHlS2yNG4DObb5BSKU=",
    version = "v1.4.3",
)

go_repository(
    name = "com_github_opencontainers_go_digest",
    importpath = "github.com/opencontainers/go-digest",
    sum = "h1:WzifXhOVOEOuFYOJAW6aQqW0TooG2iki3E3Ii+WN7gQ=",
    version = "v1.0.0-rc1",
)

go_repository(
    name = "com_github_opencontainers_image_spec",
    importpath = "github.com/opencontainers/image-spec",
    sum = "h1:JMemWkRwHx4Zj+fVxWoMCFm/8sYGGrUVojFA6h/TRcI=",
    version = "v1.0.1",
)

go_repository(
    name = "com_github_openzipkin_zipkin_go",
    importpath = "github.com/openzipkin/zipkin-go",
    sum = "h1:yXiysv1CSK7Q5yjGy1710zZGnsbMUIjluWBxtLXHPBo=",
    version = "v0.1.6",
)

go_repository(
    name = "com_github_pelletier_go_toml",
    importpath = "github.com/pelletier/go-toml",
    sum = "h1:e5+lF2E4Y2WCIxBefVowBuB0iHrUH4HZ8q+6mGF7fJc=",
    version = "v1.3.0",
)

go_repository(
    name = "com_github_peterbourgon_diskv",
    importpath = "github.com/peterbourgon/diskv",
    sum = "h1:ZKRE3mqKoxObHs5oWjLnA1WxXhmlDDAVuE0VsuLIoNk=",
    version = "v0.0.0-20171120014656-2973218375c3",
)

go_repository(
    name = "com_github_pkg_errors",
    importpath = "github.com/pkg/errors",
    sum = "h1:iURUrRGxPUNPdy5/HRSm+Yj6okJ6UtLINN0Q9M4+h3I=",
    version = "v0.8.1",
)

go_repository(
    name = "com_github_pmezard_go_difflib",
    importpath = "github.com/pmezard/go-difflib",
    sum = "h1:4DBwDE0NGyQoBHbLQYPwSUPoCMWR5BEzIk/f1lZbAQM=",
    version = "v1.0.0",
)

go_repository(
    name = "com_github_prometheus_client_golang",
    importpath = "github.com/prometheus/client_golang",
    sum = "h1:D+CiwcpGTW6pL6bv6KI3KbyEyCKyS+1JWS2h8PNDnGA=",
    version = "v0.9.3-0.20190127221311-3c4408c8b829",
)

go_repository(
    name = "com_github_prometheus_client_model",
    importpath = "github.com/prometheus/client_model",
    sum = "h1:BVwpUVJDADN2ufcGik7W992pyps0wZ888b/y9GXcLTU=",
    version = "v0.0.0-20190115171406-56726106282f",
)

go_repository(
    name = "com_github_prometheus_common",
    importpath = "github.com/prometheus/common",
    sum = "h1:kUZDBDTdBVBYBj5Tmh2NZLlF60mfjA27rM34b+cVwNU=",
    version = "v0.2.0",
)

go_repository(
    name = "com_github_prometheus_procfs",
    importpath = "github.com/prometheus/procfs",
    sum = "h1:/K3IL0Z1quvmJ7X0A1AwNEK7CRkVK3YwfOU/QAL4WGg=",
    version = "v0.0.0-20190117184657-bf6a532e95b1",
)

go_repository(
    name = "com_github_puerkitobio_purell",
    importpath = "github.com/PuerkitoBio/purell",
    sum = "h1:WEQqlqaGbrPkxLJWfBwQmfEAE1Z7ONdDLqrN38tNFfI=",
    version = "v1.1.1",
)

go_repository(
    name = "com_github_puerkitobio_urlesc",
    importpath = "github.com/PuerkitoBio/urlesc",
    sum = "h1:d+Bc7a5rLufV/sSk/8dngufqelfh6jnri85riMAaF/M=",
    version = "v0.0.0-20170810143723-de5bf2ad4578",
)

go_repository(
    name = "com_github_qor_inflection",
    importpath = "github.com/qor/inflection",
    sum = "h1:h1tL4LMzWeaTwOJrFiJJuvtlkgCPbJw1pes86e7UgeM=",
    version = "v0.0.0-20180308033659-04140366298a",
)

go_repository(
    name = "com_github_satori_go_uuid",
    importpath = "github.com/satori/go.uuid",
    sum = "h1:1r/p6yT1FfHR1+qBm7UYBPgfqCmzz/8mpNvfc+iKlfU=",
    version = "v0.0.0-20160713180306-0aa62d5ddceb",
)

go_repository(
    name = "com_github_shurcool_githubv4",
    importpath = "github.com/shurcooL/githubv4",
    sum = "h1:cppRIvEpuZcSdhbhyJZ/3ThCPYlx6xuZg8Qid/0+bz0=",
    version = "v0.0.0-20180925043049-51d7b505e2e9",
)

go_repository(
    name = "com_github_shurcool_go",
    importpath = "github.com/shurcooL/go",
    sum = "h1:MZM7FHLqUHYI0Y/mQAt3d2aYa0SiNms/hFqC9qJYolM=",
    version = "v0.0.0-20180423040247-9e1955d9fb6e",
)

go_repository(
    name = "com_github_shurcool_graphql",
    importpath = "github.com/shurcooL/graphql",
    sum = "h1:YIoQLhvoRcfiL0pyxqkESFZXa7jQrcfLTUSSUeyYMO8=",
    version = "v0.0.0-20180924043259-e4a3a37e6d42",
)

go_repository(
    name = "com_github_sirupsen_logrus",
    importpath = "github.com/sirupsen/logrus",
    sum = "h1:juTguoYk5qI21pwyTXY3B3Y5cOTH3ZUyZCg1v/mihuo=",
    version = "v1.2.0",
)

go_repository(
    name = "com_github_soheilhy_cmux",
    importpath = "github.com/soheilhy/cmux",
    sum = "h1:0HKaf1o97UwFjHH9o5XsHUOF+tqmdA7KEzXLpiyaw0E=",
    version = "v0.1.4",
)

go_repository(
    name = "com_github_spf13_cobra",
    importpath = "github.com/spf13/cobra",
    sum = "h1:ZlrZ4XsMRm04Fr5pSFxBgfND2EBVa1nLpiy1stUsX/8=",
    version = "v0.0.3",
)

go_repository(
    name = "com_github_spf13_pflag",
    importpath = "github.com/spf13/pflag",
    sum = "h1:zPAT6CGy6wXeQ7NtTnaTerfKOsV6V6F8agHXFiazDkg=",
    version = "v1.0.3",
)

go_repository(
    name = "com_github_stretchr_testify",
    importpath = "github.com/stretchr/testify",
    sum = "h1:TivCn/peBQ7UY8ooIcPgZFpTNSz0Q2U6UrFlUfqbe0Q=",
    version = "v1.3.0",
)

go_repository(
    name = "com_github_tmc_grpc_websocket_proxy",
    importpath = "github.com/tmc/grpc-websocket-proxy",
    sum = "h1:ndzgwNDnKIqyCvHTXaCqh9KlOWKvBry6nuXMJmonVsE=",
    version = "v0.0.0-20170815181823-89b8d40f7ca8",
)

go_repository(
    name = "com_github_ugorji_go",
    importpath = "github.com/ugorji/go",
    sum = "h1:gmervu+jDMvXTbcHQ0pd2wee85nEoE0BsVyEuzkfK8w=",
    version = "v1.1.1",
)

go_repository(
    name = "com_github_urfave_cli",
    importpath = "github.com/urfave/cli",
    sum = "h1:m9MfmZWX7bwr9kUcs/Asr95j0IVXzGNNc+/5ku2m26Q=",
    version = "v1.18.0",
)

go_repository(
    name = "com_github_xiang90_probing",
    importpath = "github.com/xiang90/probing",
    sum = "h1:MPPkRncZLN9Kh4MEFmbnK4h3BD7AUmskWv2+EeZJCCs=",
    version = "v0.0.0-20160813154853-07dd2e8dfe18",
)

go_repository(
    name = "com_github_xlab_handysort",
    importpath = "github.com/xlab/handysort",
    sum = "h1:j2hhcujLRHAg872RWAV5yaUrEjHEObwDv3aImCaNLek=",
    version = "v0.0.0-20150421192137-fb3537ed64a1",
)

go_repository(
    name = "com_google_cloud_go",
    importpath = "cloud.google.com/go",
    sum = "h1:glPeL3BQJsbF6aIIYfZizMwc5LTYz250bDMjttbBGAU=",
    version = "v0.37.4",
)

go_repository(
    name = "in_gopkg_airbrake_gobrake_v2",
    importpath = "gopkg.in/airbrake/gobrake.v2",
    sum = "h1:7z2uVWwn7oVeeugY1DtlPAy5H+KYgB1KeKTnqjNatLo=",
    version = "v2.0.9",
)

go_repository(
    name = "in_gopkg_check_v1",
    importpath = "gopkg.in/check.v1",
    sum = "h1:qIbj1fsPNlZgppZ+VLlY7N33q108Sa+fhmuc+sWQYwY=",
    version = "v1.0.0-20180628173108-788fd7840127",
)

go_repository(
    name = "in_gopkg_cheggaaa_pb_v1",
    importpath = "gopkg.in/cheggaaa/pb.v1",
    sum = "h1:Ev7yu1/f6+d+b3pi5vPdRPc6nNtP1umSfcWiEfRqv6I=",
    version = "v1.0.25",
)

go_repository(
    name = "in_gopkg_fsnotify_v1",
    importpath = "gopkg.in/fsnotify.v1",
    sum = "h1:xOHLXZwVvI9hhs+cLKq5+I5onOuwQLhQwiu63xxlHs4=",
    version = "v1.4.7",
)

go_repository(
    name = "in_gopkg_gemnasium_logrus_airbrake_hook_v2",
    importpath = "gopkg.in/gemnasium/logrus-airbrake-hook.v2",
    sum = "h1:OAj3g0cR6Dx/R07QgQe8wkA9RNjB2u4i700xBkIT4e0=",
    version = "v2.1.2",
)

go_repository(
    name = "in_gopkg_inf_v0",
    importpath = "gopkg.in/inf.v0",
    sum = "h1:73M5CoZyi3ZLMOyDlQh031Cx6N9NDJ2Vvfl76EDAgDc=",
    version = "v0.9.1",
)

go_repository(
    name = "in_gopkg_robfig_cron_v2",
    importpath = "gopkg.in/robfig/cron.v2",
    sum = "h1:E846t8CnR+lv5nE+VuiKTDG/v1U2stad0QzddfJC7kY=",
    version = "v2.0.0-20150107220207-be2e0b0deed5",
)

go_repository(
    name = "in_gopkg_tomb_v1",
    importpath = "gopkg.in/tomb.v1",
    sum = "h1:uRGJdciOHaEIrze2W8Q3AKkepLTh2hOroT7a+7czfdQ=",
    version = "v1.0.0-20141024135613-dd632973f1e7",
)

go_repository(
    name = "in_gopkg_yaml_v2",
    importpath = "gopkg.in/yaml.v2",
    sum = "h1:ZCJp+EgiOT7lHqUV2J862kp8Qj64Jo6az82+3Td9dZw=",
    version = "v2.2.2",
)

go_repository(
    name = "io_etcd_go_bbolt",
    importpath = "go.etcd.io/bbolt",
    sum = "h1:M0l89sIuZ+RkW0rLbUsmxescVzLwLUs+Kvks+0jeHdM=",
    version = "v1.3.1-etcd.7",
)

go_repository(
    name = "io_etcd_go_etcd",
    importpath = "go.etcd.io/etcd",
    sum = "h1:eBTQSvESFeaCGhot+9ieBlsaWyKPcJCGF/WSa+Vbrw8=",
    version = "v0.0.0-20181031231232-83304cfc808c",
)

go_repository(
    name = "io_k8s_api",
    importpath = "k8s.io/api",
    sum = "h1:yU+uHaWFaeWjRoVDuKI2qxcOP9PPFJ+665yJuHI5Ils=",
    version = "v0.0.0-20181128191700-6db15a15d2d3",
)

go_repository(
    name = "io_k8s_apiextensions_apiserver",
    importpath = "k8s.io/apiextensions-apiserver",
    sum = "h1:URFZnXg5e8m3X/WJynQspAfaFwaKKJvWWSED2+nRxKI=",
    version = "v0.0.0-20181128195303-1f84094d7e8e",
)

go_repository(
    name = "io_k8s_apimachinery",
    importpath = "k8s.io/apimachinery",
    sum = "h1:DRtb2PO1ps3SPBkAUYz9J+rIPvYDZALFAYhWBeyvmcU=",
    version = "v0.0.0-20181128191346-49ce2735e507",
)

go_repository(
    name = "io_k8s_client_go",
    importpath = "k8s.io/client-go",
    sum = "h1:2kqW3X2xQ9SbFvWZjGEHBLlWc1LG9JIJNXWkuqwdZ3A=",
    version = "v9.0.0+incompatible",
)

go_repository(
    name = "io_k8s_kube_openapi",
    importpath = "k8s.io/kube-openapi",
    sum = "h1:mn2F9UzCk6KGa7M/d2ibLyRtBQm7n6QvbCjDe/cDWSg=",
    version = "v0.0.0-20180711000925-0cf8f7e6ed1d",
)

go_repository(
    name = "io_k8s_sigs_yaml",
    importpath = "sigs.k8s.io/yaml",
    sum = "h1:4A07+ZFc2wgJwo8YNlQpr1rVlgUDlxXHhPJciaPY5gs=",
    version = "v1.1.0",
)

go_repository(
    name = "io_k8s_utils",
    importpath = "k8s.io/utils",
    sum = "h1:FVNQlf0b+xgJl9ko2aJXH3wqMfny57f5oSKJcnyRkL8=",
    version = "v0.0.0-20181019225348-5e321f9a457c",
)

go_repository(
    name = "io_opencensus_go",
    importpath = "go.opencensus.io",
    sum = "h1:NAfh7zF0/3/HqtMvJNZ/RFrSlCE6ZTlHmKfhL/Dm1Jk=",
    version = "v0.20.2",
)

go_repository(
    name = "ml_vbom_util",
    importpath = "vbom.ml/util",
    sum = "h1:VRvrHgbw/Yg8mlNgD20TTQsTXTNJenU4jFUEqVxDRCU=",
    version = "v0.0.0-20170409195630-256737ac55c4",
)

go_repository(
    name = "org_apache_git_thrift_git",
    importpath = "git.apache.org/thrift.git",
    sum = "h1:CMxsZlAmxKs+VAZMlDDL0wXciMblJcutQbEe3A9CYUM=",
    version = "v0.12.0",
)

go_repository(
    name = "org_golang_google_api",
    importpath = "google.golang.org/api",
    sum = "h1:iTp+3yyl/KOtxa/d1/JUE0GGSoR6FuW5udver22iwpw=",
    version = "v0.3.2",
)

go_repository(
    name = "org_golang_google_appengine",
    importpath = "google.golang.org/appengine",
    sum = "h1:/wp5JvzpHIxhs/dumFmF7BXTf3Z+dd4uXta4kVyO508=",
    version = "v1.4.0",
)

go_repository(
    name = "org_golang_google_genproto",
    importpath = "google.golang.org/genproto",
    sum = "h1:xtNn7qFlagY2mQNFHMSRPjT2RkOV4OXM7P5TVy9xATo=",
    version = "v0.0.0-20190404172233-64821d5d2107",
)

go_repository(
    name = "org_golang_google_grpc",
    importpath = "google.golang.org/grpc",
    sum = "h1:TrBcJ1yqAl1G++wO39nD/qtgpsW9/1+QGrluyMGEYgM=",
    version = "v1.19.1",
)

go_repository(
    name = "org_golang_x_crypto",
    importpath = "golang.org/x/crypto",
    sum = "h1:bselrhR0Or1vomJZC8ZIjWtbDmn9OYFLX5Ik9alpJpE=",
    version = "v0.0.0-20190404164418-38d8ce5564a5",
)

go_repository(
    name = "org_golang_x_lint",
    importpath = "golang.org/x/lint",
    sum = "h1:hX65Cu3JDlGH3uEdK7I99Ii+9kjD6mvnnpfLdEAH0x4=",
    version = "v0.0.0-20190301231843-5614ed5bae6f",
)

go_repository(
    name = "org_golang_x_net",
    importpath = "golang.org/x/net",
    sum = "h1:/zjbcJPEGAyu6Is/VBOALsgdi4z9+kz/Vtdm6S+beD0=",
    version = "v0.0.0-20190403144856-b630fd6fe46b",
)

go_repository(
    name = "org_golang_x_oauth2",
    importpath = "golang.org/x/oauth2",
    sum = "h1:Wo7BWFiOk0QRFMLYMqJGFMd9CgUAcGx7V+qEg/h5IBI=",
    version = "v0.0.0-20190226205417-e64efc72b421",
)

go_repository(
    name = "org_golang_x_sync",
    importpath = "golang.org/x/sync",
    sum = "h1:bjcUS9ztw9kFmmIxJInhon/0Is3p+EHBKNgquIzo1OI=",
    version = "v0.0.0-20190227155943-e225da77a7e6",
)

go_repository(
    name = "org_golang_x_sys",
    importpath = "golang.org/x/sys",
    sum = "h1:nFYrTHrdrAOpShe27kaFHjsqYSEQ0KWqdWLu3xuZJts=",
    version = "v0.0.0-20190403152447-81d4e9dc473e",
)

go_repository(
    name = "org_golang_x_text",
    importpath = "golang.org/x/text",
    sum = "h1:tW2bmiBqwgJj/UpqtC8EpXEZVYOwU0yG4iWbprSVAcs=",
    version = "v0.3.2",
)

go_repository(
    name = "org_golang_x_time",
    importpath = "golang.org/x/time",
    sum = "h1:fqgJT0MGcGpPgpWU7VRdRjuArfcOvC4AoJmILihzhDg=",
    version = "v0.0.0-20181108054448-85acf8d2951c",
)

go_repository(
    name = "org_golang_x_tools",
    importpath = "golang.org/x/tools",
    sum = "h1:MtqjsZmyGRgMmLUgxnmMJ6RYdvd2ib8ipiayHhqSxs4=",
    version = "v0.0.0-20190404132500-923d25813098",
)

go_repository(
    name = "org_uber_go_atomic",
    importpath = "go.uber.org/atomic",
    sum = "h1:2Oa65PReHzfn29GpvgsYwloV9AVFHPDk8tYxt2c2tr4=",
    version = "v1.3.2",
)

go_repository(
    name = "org_uber_go_multierr",
    importpath = "go.uber.org/multierr",
    sum = "h1:HoEmRHQPVSqub6w2z2d2EOVs2fjyFRGyofhKuyDq0QI=",
    version = "v1.1.0",
)

go_repository(
    name = "org_uber_go_zap",
    importpath = "go.uber.org/zap",
    sum = "h1:XCJQEf3W6eZaVwhRBof6ImoYGJSITeKWsyeh3HFu/5o=",
    version = "v1.9.1",
)

go_repository(
    name = "io_k8s_klog",
    importpath = "k8s.io/klog",
    sum = "h1:I5HMfc/DtuVaGR1KPwUrTc476K8NCqNBldC7H4dYEzk=",
    version = "v0.1.0",
)

go_repository(
    name = "cc_mvdan_xurls_v2",
    importpath = "mvdan.cc/xurls/v2",
    sum = "h1:r1zSOSNS/kqtpmATyMMMvaZ4/djsesbYz5kr0+qMRWc=",
    version = "v2.0.0",
)

go_repository(
    name = "io_k8s_code_generator",
    importpath = "k8s.io/code-generator",
    sum = "h1:piQj4auUdxujSfNkNdqNCG3Udqxe5RqHe/zVN5uFbmE=",
    version = "v0.0.0-20190301173042-b1289fc74931",
)

go_repository(
    name = "io_k8s_gengo",
    importpath = "k8s.io/gengo",
    sum = "h1:hgREZo/BX3MNhKzDN8oWk8+YM7SEwB0AjGHoo+BHkyQ=",
    version = "v0.0.0-20190306031000-7a1b7fb0289f",
)

go_repository(
    name = "com_github_kisielk_errcheck",
    importpath = "github.com/kisielk/errcheck",
    sum = "h1:ZqfnKyx9KGpRcW04j5nnPDgRgoXUeLh2YFBeFzphcA0=",
    version = "v1.1.0",
)

go_repository(
    name = "com_github_alecthomas_template",
    importpath = "github.com/alecthomas/template",
    sum = "h1:cAKDfWh5VpdgMhJosfJnn5/FoN2SRZ4p7fJNX58YPaU=",
    version = "v0.0.0-20160405071501-a0175ee3bccc",
)

go_repository(
    name = "com_github_alecthomas_units",
    importpath = "github.com/alecthomas/units",
    sum = "h1:qet1QNfXsQxTZqLG4oE62mJzwPIB8+Tee4RNCL9ulrY=",
    version = "v0.0.0-20151022065526-2efee857e7cf",
)

go_repository(
    name = "com_github_anmitsu_go_shlex",
    importpath = "github.com/anmitsu/go-shlex",
    sum = "h1:kFOfPq6dUM1hTo4JG6LR5AXSUEsOjtdm0kw0FtQtMJA=",
    version = "v0.0.0-20161002113705-648efa622239",
)

go_repository(
    name = "com_github_bradfitz_go_smtpd",
    importpath = "github.com/bradfitz/go-smtpd",
    sum = "h1:ckJgFhFWywOx+YLEMIJsTb+NV6NexWICk5+AMSuz3ss=",
    version = "v0.0.0-20170404230938-deb6d6237625",
)

go_repository(
    name = "com_github_flynn_go_shlex",
    importpath = "github.com/flynn/go-shlex",
    sum = "h1:BHsljHzVlRcyQhjrss6TZTdY2VfCqZPbv5k3iBFa2ZQ=",
    version = "v0.0.0-20150515145356-3f9db97f8568",
)

go_repository(
    name = "com_github_gliderlabs_ssh",
    importpath = "github.com/gliderlabs/ssh",
    sum = "h1:j3L6gSLQalDETeEg/Jg0mGY0/y/N6zI2xX1978P0Uqw=",
    version = "v0.1.1",
)

go_repository(
    name = "com_github_go_kit_kit",
    importpath = "github.com/go-kit/kit",
    sum = "h1:Wz+5lgoB0kkuqLEc6NVmwRknTKP6dTGbSqvhZtBI/j0=",
    version = "v0.8.0",
)

go_repository(
    name = "com_github_go_logfmt_logfmt",
    importpath = "github.com/go-logfmt/logfmt",
    sum = "h1:8HUsc87TaSWLKwrnumgC8/YconD2fJQsRJAsWaPg2ic=",
    version = "v0.3.0",
)

go_repository(
    name = "com_github_go_stack_stack",
    importpath = "github.com/go-stack/stack",
    sum = "h1:5SgMzNM5HxrEjV0ww2lTmX6E2Izsfxas4+YHWRs3Lsk=",
    version = "v1.8.0",
)

go_repository(
    name = "com_github_google_pprof",
    importpath = "github.com/google/pprof",
    sum = "h1:eqyIo2HjKhKe/mJzTG8n4VqvLXIOEG+SLdDqX7xGtkY=",
    version = "v0.0.0-20181206194817-3ea8567a2e57",
)

go_repository(
    name = "com_github_googleapis_gax_go_v2",
    importpath = "github.com/googleapis/gax-go/v2",
    sum = "h1:hU4mGcQI4DaAYW+IbTun+2qEZVFxK0ySjQLTbS0VQKc=",
    version = "v2.0.4",
)

go_repository(
    name = "com_github_jellevandenhooff_dkim",
    importpath = "github.com/jellevandenhooff/dkim",
    sum = "h1:ujPKutqRlJtcfWk6toYVYagwra7HQHbXOaS171b4Tg8=",
    version = "v0.0.0-20150330215556-f50fe3d243e1",
)

go_repository(
    name = "com_github_jstemmer_go_junit_report",
    importpath = "github.com/jstemmer/go-junit-report",
    sum = "h1:rBMNdlhTLzJjJSDIjNEXX1Pz3Hmwmz91v+zycvx9PJc=",
    version = "v0.0.0-20190106144839-af01ea7f8024",
)

go_repository(
    name = "com_github_julienschmidt_httprouter",
    importpath = "github.com/julienschmidt/httprouter",
    sum = "h1:TDTW5Yz1mjftljbcKqRcrYhd4XeOoI98t+9HbQbYf7g=",
    version = "v1.2.0",
)

go_repository(
    name = "com_github_klauspost_compress",
    importpath = "github.com/klauspost/compress",
    sum = "h1:8VMb5+0wMgdBykOV96DwNwKFQ+WTI4pzYURP99CcB9E=",
    version = "v1.4.1",
)

go_repository(
    name = "com_github_klauspost_cpuid",
    importpath = "github.com/klauspost/cpuid",
    sum = "h1:vJi+O/nMdFt0vqm8NZBI6wzALWdA2X+egi0ogNyrC/w=",
    version = "v1.2.1",
)

go_repository(
    name = "com_github_kr_logfmt",
    importpath = "github.com/kr/logfmt",
    sum = "h1:T+h1c/A9Gawja4Y9mFVWj2vyii2bbUNDw3kt9VxK2EY=",
    version = "v0.0.0-20140226030751-b84e30acd515",
)

go_repository(
    name = "com_github_kr_pretty",
    importpath = "github.com/kr/pretty",
    sum = "h1:L/CwN0zerZDmRFUapSPitk6f+Q3+0za1rQkzVuMiMFI=",
    version = "v0.1.0",
)

go_repository(
    name = "com_github_kr_text",
    importpath = "github.com/kr/text",
    sum = "h1:45sCR5RtlFHMR4UwH9sdQ5TC8v0qDQCHnXt+kaKSTVE=",
    version = "v0.1.0",
)

go_repository(
    name = "com_github_mwitkow_go_conntrack",
    importpath = "github.com/mwitkow/go-conntrack",
    sum = "h1:F9x/1yl3T2AeKLr2AMdilSD8+f9bvMnNN8VS5iDtovc=",
    version = "v0.0.0-20161129095857-cc309e4a2223",
)

go_repository(
    name = "com_github_stretchr_objx",
    importpath = "github.com/stretchr/objx",
    sum = "h1:2vfRuCMp5sSVIDSqO8oNnWJq7mPa6KVP3iPIwFBuy8A=",
    version = "v0.1.1",
)

go_repository(
    name = "com_github_tarm_serial",
    importpath = "github.com/tarm/serial",
    sum = "h1:UyzmZLoiDWMRywV4DUYb9Fbt8uiOSooupjTq10vpvnU=",
    version = "v0.0.0-20180830185346-98f6abe2eb07",
)

go_repository(
    name = "in_gopkg_alecthomas_kingpin_v2",
    importpath = "gopkg.in/alecthomas/kingpin.v2",
    sum = "h1:jMFz6MfLP0/4fUyZle81rXUoxOBFi19VUFKVDOQfozc=",
    version = "v2.2.6",
)

go_repository(
    name = "org_go4",
    importpath = "go4.org",
    sum = "h1:+hE86LblG4AyDgwMCLTE6FOlM9+qjHSYS+rKqxUVdsM=",
    version = "v0.0.0-20180809161055-417644f6feb5",
)

go_repository(
    name = "org_go4_grpc",
    importpath = "grpc.go4.org",
    sum = "h1:tmXTu+dfa+d9Evp8NpJdgOy6+rt8/x4yG7qPBrtNfLY=",
    version = "v0.0.0-20170609214715-11d0a25b4919",
)

go_repository(
    name = "org_golang_x_build",
    importpath = "golang.org/x/build",
    sum = "h1:9vRy8wdKITrvvXLEOnNC9FHAGhmzy3OwvKfscMgJ4vo=",
    version = "v0.0.0-20190314133821-5284462c4bec",
)

go_repository(
    name = "org_golang_x_exp",
    importpath = "golang.org/x/exp",
    sum = "h1:c2HOrn5iMezYjSlGPncknSEr/8x5LELb/ilJbXi9DEA=",
    version = "v0.0.0-20190121172915-509febef88a4",
)

go_repository(
    name = "org_golang_x_perf",
    importpath = "golang.org/x/perf",
    sum = "h1:xYq6+9AtI+xP3M4r0N1hCkHrInHDBohhquRgx9Kk6gI=",
    version = "v0.0.0-20180704124530-6e6d33e29852",
)

go_repository(
    name = "com_github_bazelbuild_bazel_gazelle",
    importpath = "github.com/bazelbuild/bazel-gazelle",
    sum = "h1:WMSKgGq+yTC1EwJDM/qM27cUpm6HFR59K1qHrz9+Lps=",
    version = "v0.0.0-20190402225339-e530fae7ce5c",
)

go_repository(
    name = "com_github_apache_thrift",
    importpath = "github.com/apache/thrift",
    sum = "h1:pODnxUFNcjP9UTLZGTdeh+j16A8lJbRvD3rOtrk/7bs=",
    version = "v0.12.0",
)

go_repository(
    name = "com_github_eapache_go_resiliency",
    importpath = "github.com/eapache/go-resiliency",
    sum = "h1:1NtRmCAqadE2FN4ZcN6g90TP3uk8cg9rn9eNK2197aU=",
    version = "v1.1.0",
)

go_repository(
    name = "com_github_eapache_go_xerial_snappy",
    importpath = "github.com/eapache/go-xerial-snappy",
    sum = "h1:YEetp8/yCZMuEPMUDHG0CW/brkkEp8mzqk2+ODEitlw=",
    version = "v0.0.0-20180814174437-776d5712da21",
)

go_repository(
    name = "com_github_eapache_queue",
    importpath = "github.com/eapache/queue",
    sum = "h1:YOEu7KNc61ntiQlcEeUIoDTJ2o8mQznoNvUhiigpIqc=",
    version = "v1.1.0",
)

go_repository(
    name = "com_github_golang_snappy",
    importpath = "github.com/golang/snappy",
    sum = "h1:woRePGFeVFfLKN/pOkfl+p/TAqKOfFu+7KPlMVpok/w=",
    version = "v0.0.0-20180518054509-2e65f85255db",
)

go_repository(
    name = "com_github_pierrec_lz4",
    importpath = "github.com/pierrec/lz4",
    sum = "h1:2xWsjqPFWcplujydGg4WmhC/6fZqK42wMM8aXeqhl0I=",
    version = "v2.0.5+incompatible",
)

go_repository(
    name = "com_github_rcrowley_go_metrics",
    importpath = "github.com/rcrowley/go-metrics",
    sum = "h1:9ZKAASQSHhDYGoxY8uLVpewe1GDZ2vu2Tr/vTdVAkFQ=",
    version = "v0.0.0-20181016184325-3113b8401b8a",
)

go_repository(
    name = "com_github_shopify_sarama",
    importpath = "github.com/Shopify/sarama",
    sum = "h1:9oksLxC6uxVPHPVYUmq6xhr1BOF/hHobWH2UzO67z1s=",
    version = "v1.19.0",
)

go_repository(
    name = "com_github_shopify_toxiproxy",
    importpath = "github.com/Shopify/toxiproxy",
    sum = "h1:TKdv8HiTLgE5wdJuEML90aBgNWsokNbMijUGhmcoBJc=",
    version = "v2.1.4+incompatible",
)

go_repository(
    name = "com_github_klauspost_pgzip",
    importpath = "github.com/klauspost/pgzip",
    sum = "h1:oIPZROsWuPHpOdMVWLuJZXwgjhrW8r1yEX8UqMyeNHM=",
    version = "v1.2.1",
)

go_repository(
    name = "com_github_gobuffalo_envy",
    importpath = "github.com/gobuffalo/envy",
    sum = "h1:OsV5vOpHYUpP7ZLS6sem1y40/lNX1BZj+ynMiRi21lQ=",
    version = "v1.6.15",
)

go_repository(
    name = "com_github_google_go_containerregistry",
    importpath = "github.com/google/go-containerregistry",
    sum = "h1:p6kvQYhOQUpogpCn1p/XI/A9UClYVyNTkYkKIijnl7M=",
    version = "v0.0.0-20190401215819-f1df91a4a813",
)

go_repository(
    name = "com_github_joho_godotenv",
    importpath = "github.com/joho/godotenv",
    sum = "h1:Zjp+RcGpHhGlrMbJzXTrZZPrWj+1vfm90La1wgB6Bhc=",
    version = "v1.3.0",
)

go_repository(
    name = "com_github_markbates_inflect",
    importpath = "github.com/markbates/inflect",
    sum = "h1:5fh1gzTFhfae06u3hzHYO9xe3l3v3nW5Pwt3naLTP5g=",
    version = "v1.0.4",
)

go_repository(
    name = "com_github_tektoncd_pipeline",
    importpath = "github.com/tektoncd/pipeline",
    sum = "h1:qxPdNf3noTDiB1IhLJk5MK7giWPk36oXF6LObILlbIk=",
    version = "v0.1.1-0.20190327171839-7c43fbae2816",
)

go_repository(
    name = "io_k8s_kubernetes",
    importpath = "k8s.io/kubernetes",
    sum = "h1:I9F52h5sqVxBmoSsBlNQ0YygNcukDilkpGxUbJRoBoY=",
    version = "v1.14.1",
)

go_repository(
    name = "com_github_rogpeppe_go_internal",
    importpath = "github.com/rogpeppe/go-internal",
    sum = "h1:g0fH8RicVgNl+zVZDCDfbdWxAWoAEJyI7I3TZYXFiig=",
    version = "v1.1.0",
)

go_repository(
    name = "in_gopkg_errgo_v2",
    importpath = "gopkg.in/errgo.v2",
    sum = "h1:0vLT13EuvQ0hNvakwLuFZ/jYrLp5F3kcWHXdRggjCE8=",
    version = "v2.1.0",
)
