load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = [
        "appengine.go",
        "appengine_vm.go",
        "errors.go",
        "identity.go",
        "namespace.go",
        "timeout.go",
    ],
    importpath = "google.golang.org/appengine",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/golang/protobuf/proto:go_default_library",
        "//vendor/golang.org/x/net/context:go_default_library",
        "//vendor/google.golang.org/appengine/internal:go_default_library",
        "//vendor/google.golang.org/appengine/internal/app_identity:go_default_library",
        "//vendor/google.golang.org/appengine/internal/modules:go_default_library",
    ],
)

go_test(
    name = "go_default_test",
    srcs = [
        "appengine_test.go",
        "namespace_test.go",
    ],
    importpath = "google.golang.org/appengine",
    library = ":go_default_library",
    deps = ["//vendor/golang.org/x/net/context:go_default_library"],
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
        "//vendor/google.golang.org/appengine/internal:all-srcs",
        "//vendor/google.golang.org/appengine/urlfetch:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
