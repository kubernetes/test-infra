load("@io_bazel_rules_go//go:def.bzl", "go_library")

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
        "//vendor/google.golang.org/appengine/aetest:all-srcs",
        "//vendor/google.golang.org/appengine/blobstore:all-srcs",
        "//vendor/google.golang.org/appengine/capability:all-srcs",
        "//vendor/google.golang.org/appengine/channel:all-srcs",
        "//vendor/google.golang.org/appengine/cloudsql:all-srcs",
        "//vendor/google.golang.org/appengine/cmd/aebundler:all-srcs",
        "//vendor/google.golang.org/appengine/cmd/aedeploy:all-srcs",
        "//vendor/google.golang.org/appengine/cmd/aefix:all-srcs",
        "//vendor/google.golang.org/appengine/datastore:all-srcs",
        "//vendor/google.golang.org/appengine/delay:all-srcs",
        "//vendor/google.golang.org/appengine/demos/guestbook:all-srcs",
        "//vendor/google.golang.org/appengine/demos/helloworld:all-srcs",
        "//vendor/google.golang.org/appengine/file:all-srcs",
        "//vendor/google.golang.org/appengine/image:all-srcs",
        "//vendor/google.golang.org/appengine/internal:all-srcs",
        "//vendor/google.golang.org/appengine/log:all-srcs",
        "//vendor/google.golang.org/appengine/mail:all-srcs",
        "//vendor/google.golang.org/appengine/memcache:all-srcs",
        "//vendor/google.golang.org/appengine/module:all-srcs",
        "//vendor/google.golang.org/appengine/remote_api:all-srcs",
        "//vendor/google.golang.org/appengine/runtime:all-srcs",
        "//vendor/google.golang.org/appengine/search:all-srcs",
        "//vendor/google.golang.org/appengine/socket:all-srcs",
        "//vendor/google.golang.org/appengine/taskqueue:all-srcs",
        "//vendor/google.golang.org/appengine/urlfetch:all-srcs",
        "//vendor/google.golang.org/appengine/user:all-srcs",
        "//vendor/google.golang.org/appengine/xmpp:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
