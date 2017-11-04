load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = [
        "association.go",
        "callback.go",
        "callback_create.go",
        "callback_delete.go",
        "callback_query.go",
        "callback_query_preload.go",
        "callback_row_query.go",
        "callback_save.go",
        "callback_update.go",
        "dialect.go",
        "dialect_common.go",
        "dialect_mysql.go",
        "dialect_postgres.go",
        "dialect_sqlite3.go",
        "errors.go",
        "field.go",
        "interface.go",
        "join_table_handler.go",
        "logger.go",
        "main.go",
        "model.go",
        "model_struct.go",
        "scope.go",
        "search.go",
        "utils.go",
    ],
    importpath = "github.com/jinzhu/gorm",
    visibility = ["//visibility:public"],
    deps = ["//vendor/github.com/jinzhu/inflection:go_default_library"],
)

go_test(
    name = "go_default_test",
    srcs = [
        "callback_system_test.go",
        "search_test.go",
    ],
    importpath = "github.com/jinzhu/gorm",
    library = ":go_default_library",
)

go_test(
    name = "go_default_xtest",
    srcs = [
        "association_test.go",
        "callbacks_test.go",
        "create_test.go",
        "customize_column_test.go",
        "delete_test.go",
        "embedded_struct_test.go",
        "errors_test.go",
        "field_test.go",
        "join_table_test.go",
        "main_test.go",
        "migration_test.go",
        "multi_primary_keys_test.go",
        "pointer_test.go",
        "polymorphic_test.go",
        "preload_test.go",
        "query_test.go",
        "scaner_test.go",
        "scope_test.go",
        "update_test.go",
        "utils_test.go",
    ],
    importpath = "github.com/jinzhu/gorm_test",
    deps = [
        ":go_default_library",
        "//vendor/github.com/erikstmartin/go-testdb:go_default_library",
        "//vendor/github.com/jinzhu/gorm/dialects/mssql:go_default_library",
        "//vendor/github.com/jinzhu/gorm/dialects/mysql:go_default_library",
        "//vendor/github.com/jinzhu/gorm/dialects/postgres:go_default_library",
        "//vendor/github.com/jinzhu/gorm/dialects/sqlite:go_default_library",
        "//vendor/github.com/jinzhu/now:go_default_library",
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
        "//vendor/github.com/jinzhu/gorm/dialects/mysql:all-srcs",
        "//vendor/github.com/jinzhu/gorm/dialects/sqlite:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
