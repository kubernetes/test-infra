package(default_visibility = ["//visibility:public"])

licenses(["notice"])

load(
    "@io_bazel_rules_go//go:def.bzl",
    "cgo_genrule",
    "go_library",
)

go_library(
    name = "github.com/NYTimes/gziphandler",
    srcs = ["github.com/NYTimes/gziphandler/gzip.go"],
    tags = ["automanaged"],
)

go_library(
    name = "github.com/Sirupsen/logrus",
    srcs = [
        "github.com/Sirupsen/logrus/alt_exit.go",
        "github.com/Sirupsen/logrus/doc.go",
        "github.com/Sirupsen/logrus/entry.go",
        "github.com/Sirupsen/logrus/exported.go",
        "github.com/Sirupsen/logrus/formatter.go",
        "github.com/Sirupsen/logrus/hooks.go",
        "github.com/Sirupsen/logrus/json_formatter.go",
        "github.com/Sirupsen/logrus/logger.go",
        "github.com/Sirupsen/logrus/logrus.go",
        "github.com/Sirupsen/logrus/terminal_linux.go",
        "github.com/Sirupsen/logrus/terminal_notwindows.go",
        "github.com/Sirupsen/logrus/text_formatter.go",
        "github.com/Sirupsen/logrus/writer.go",
    ],
    tags = ["automanaged"],
)

go_library(
    name = "github.com/ghodss/yaml",
    srcs = [
        "github.com/ghodss/yaml/fields.go",
        "github.com/ghodss/yaml/yaml.go",
    ],
    tags = ["automanaged"],
    deps = ["//vendor:gopkg.in/yaml.v2"],
)

go_library(
    name = "github.com/go-sql-driver/mysql",
    srcs = [
        "github.com/go-sql-driver/mysql/buffer.go",
        "github.com/go-sql-driver/mysql/collations.go",
        "github.com/go-sql-driver/mysql/connection.go",
        "github.com/go-sql-driver/mysql/const.go",
        "github.com/go-sql-driver/mysql/driver.go",
        "github.com/go-sql-driver/mysql/dsn.go",
        "github.com/go-sql-driver/mysql/errors.go",
        "github.com/go-sql-driver/mysql/infile.go",
        "github.com/go-sql-driver/mysql/packets.go",
        "github.com/go-sql-driver/mysql/result.go",
        "github.com/go-sql-driver/mysql/rows.go",
        "github.com/go-sql-driver/mysql/statement.go",
        "github.com/go-sql-driver/mysql/transaction.go",
        "github.com/go-sql-driver/mysql/utils.go",
    ],
    tags = ["automanaged"],
)

go_library(
    name = "github.com/golang/glog",
    srcs = [
        "github.com/golang/glog/glog.go",
        "github.com/golang/glog/glog_file.go",
    ],
    tags = ["automanaged"],
)

go_library(
    name = "github.com/golang/mock/gomock",
    srcs = [
        "github.com/golang/mock/gomock/call.go",
        "github.com/golang/mock/gomock/callset.go",
        "github.com/golang/mock/gomock/controller.go",
        "github.com/golang/mock/gomock/matchers.go",
    ],
    tags = ["automanaged"],
)

go_library(
    name = "github.com/golang/protobuf/proto",
    srcs = [
        "github.com/golang/protobuf/proto/clone.go",
        "github.com/golang/protobuf/proto/decode.go",
        "github.com/golang/protobuf/proto/encode.go",
        "github.com/golang/protobuf/proto/equal.go",
        "github.com/golang/protobuf/proto/extensions.go",
        "github.com/golang/protobuf/proto/lib.go",
        "github.com/golang/protobuf/proto/message_set.go",
        "github.com/golang/protobuf/proto/pointer_unsafe.go",
        "github.com/golang/protobuf/proto/properties.go",
        "github.com/golang/protobuf/proto/text.go",
        "github.com/golang/protobuf/proto/text_parser.go",
    ],
    tags = ["automanaged"],
)

go_library(
    name = "github.com/google/go-github/github",
    srcs = [
        "github.com/google/go-github/github/activity.go",
        "github.com/google/go-github/github/activity_events.go",
        "github.com/google/go-github/github/activity_notifications.go",
        "github.com/google/go-github/github/activity_star.go",
        "github.com/google/go-github/github/activity_watching.go",
        "github.com/google/go-github/github/authorizations.go",
        "github.com/google/go-github/github/doc.go",
        "github.com/google/go-github/github/event_types.go",
        "github.com/google/go-github/github/gists.go",
        "github.com/google/go-github/github/gists_comments.go",
        "github.com/google/go-github/github/git.go",
        "github.com/google/go-github/github/git_blobs.go",
        "github.com/google/go-github/github/git_commits.go",
        "github.com/google/go-github/github/git_refs.go",
        "github.com/google/go-github/github/git_tags.go",
        "github.com/google/go-github/github/git_trees.go",
        "github.com/google/go-github/github/github.go",
        "github.com/google/go-github/github/gitignore.go",
        "github.com/google/go-github/github/issues.go",
        "github.com/google/go-github/github/issues_assignees.go",
        "github.com/google/go-github/github/issues_comments.go",
        "github.com/google/go-github/github/issues_events.go",
        "github.com/google/go-github/github/issues_labels.go",
        "github.com/google/go-github/github/issues_milestones.go",
        "github.com/google/go-github/github/issues_timeline.go",
        "github.com/google/go-github/github/licenses.go",
        "github.com/google/go-github/github/messages.go",
        "github.com/google/go-github/github/migrations.go",
        "github.com/google/go-github/github/migrations_source_import.go",
        "github.com/google/go-github/github/misc.go",
        "github.com/google/go-github/github/orgs.go",
        "github.com/google/go-github/github/orgs_hooks.go",
        "github.com/google/go-github/github/orgs_members.go",
        "github.com/google/go-github/github/orgs_teams.go",
        "github.com/google/go-github/github/pulls.go",
        "github.com/google/go-github/github/pulls_comments.go",
        "github.com/google/go-github/github/reactions.go",
        "github.com/google/go-github/github/repos.go",
        "github.com/google/go-github/github/repos_collaborators.go",
        "github.com/google/go-github/github/repos_comments.go",
        "github.com/google/go-github/github/repos_commits.go",
        "github.com/google/go-github/github/repos_contents.go",
        "github.com/google/go-github/github/repos_deployments.go",
        "github.com/google/go-github/github/repos_forks.go",
        "github.com/google/go-github/github/repos_hooks.go",
        "github.com/google/go-github/github/repos_invitations.go",
        "github.com/google/go-github/github/repos_keys.go",
        "github.com/google/go-github/github/repos_merging.go",
        "github.com/google/go-github/github/repos_pages.go",
        "github.com/google/go-github/github/repos_releases.go",
        "github.com/google/go-github/github/repos_stats.go",
        "github.com/google/go-github/github/repos_statuses.go",
        "github.com/google/go-github/github/search.go",
        "github.com/google/go-github/github/strings.go",
        "github.com/google/go-github/github/timestamp.go",
        "github.com/google/go-github/github/users.go",
        "github.com/google/go-github/github/users_administration.go",
        "github.com/google/go-github/github/users_emails.go",
        "github.com/google/go-github/github/users_followers.go",
        "github.com/google/go-github/github/users_gpg_keys.go",
        "github.com/google/go-github/github/users_keys.go",
    ],
    tags = ["automanaged"],
    deps = ["//vendor:github.com/google/go-querystring/query"],
)

go_library(
    name = "github.com/google/go-querystring/query",
    srcs = ["github.com/google/go-querystring/query/encode.go"],
    tags = ["automanaged"],
)

go_library(
    name = "github.com/inconshreveable/mousetrap",
    srcs = ["github.com/inconshreveable/mousetrap/trap_others.go"],
    tags = ["automanaged"],
)

go_library(
    name = "github.com/influxdata/influxdb/client/v2",
    srcs = [
        "github.com/influxdata/influxdb/client/v2/client.go",
        "github.com/influxdata/influxdb/client/v2/udp.go",
    ],
    tags = ["automanaged"],
    deps = ["//vendor:github.com/influxdata/influxdb/models"],
)

go_library(
    name = "github.com/influxdata/influxdb/models",
    srcs = [
        "github.com/influxdata/influxdb/models/consistency.go",
        "github.com/influxdata/influxdb/models/inline_fnv.go",
        "github.com/influxdata/influxdb/models/inline_strconv_parse.go",
        "github.com/influxdata/influxdb/models/points.go",
        "github.com/influxdata/influxdb/models/rows.go",
        "github.com/influxdata/influxdb/models/statistic.go",
        "github.com/influxdata/influxdb/models/time.go",
    ],
    tags = ["automanaged"],
    deps = ["//vendor:github.com/influxdata/influxdb/pkg/escape"],
)

go_library(
    name = "github.com/influxdata/influxdb/pkg/escape",
    srcs = [
        "github.com/influxdata/influxdb/pkg/escape/bytes.go",
        "github.com/influxdata/influxdb/pkg/escape/strings.go",
    ],
    tags = ["automanaged"],
)

go_library(
    name = "github.com/jinzhu/gorm",
    srcs = [
        "github.com/jinzhu/gorm/association.go",
        "github.com/jinzhu/gorm/callback.go",
        "github.com/jinzhu/gorm/callback_create.go",
        "github.com/jinzhu/gorm/callback_delete.go",
        "github.com/jinzhu/gorm/callback_query.go",
        "github.com/jinzhu/gorm/callback_query_preload.go",
        "github.com/jinzhu/gorm/callback_save.go",
        "github.com/jinzhu/gorm/callback_update.go",
        "github.com/jinzhu/gorm/dialect.go",
        "github.com/jinzhu/gorm/dialect_common.go",
        "github.com/jinzhu/gorm/dialect_mysql.go",
        "github.com/jinzhu/gorm/dialect_postgres.go",
        "github.com/jinzhu/gorm/dialect_sqlite3.go",
        "github.com/jinzhu/gorm/errors.go",
        "github.com/jinzhu/gorm/field.go",
        "github.com/jinzhu/gorm/interface.go",
        "github.com/jinzhu/gorm/join_table_handler.go",
        "github.com/jinzhu/gorm/logger.go",
        "github.com/jinzhu/gorm/main.go",
        "github.com/jinzhu/gorm/model.go",
        "github.com/jinzhu/gorm/model_struct.go",
        "github.com/jinzhu/gorm/scope.go",
        "github.com/jinzhu/gorm/search.go",
        "github.com/jinzhu/gorm/utils.go",
    ],
    tags = ["automanaged"],
    deps = ["//vendor:github.com/jinzhu/inflection"],
)

go_library(
    name = "github.com/jinzhu/gorm/dialects/mysql",
    srcs = ["github.com/jinzhu/gorm/dialects/mysql/mysql.go"],
    tags = ["automanaged"],
    deps = ["//vendor:github.com/go-sql-driver/mysql"],
)

go_library(
    name = "github.com/jinzhu/gorm/dialects/sqlite",
    srcs = ["github.com/jinzhu/gorm/dialects/sqlite/sqlite.go"],
    tags = ["automanaged"],
    deps = ["//vendor:github.com/mattn/go-sqlite3"],
)

go_library(
    name = "github.com/jinzhu/inflection",
    srcs = ["github.com/jinzhu/inflection/inflections.go"],
    tags = ["automanaged"],
)

cgo_genrule(
    name = "github.com/mattn/go-sqlite3_cgo",
    srcs = [
        "github.com/mattn/go-sqlite3/backup.go",
        "github.com/mattn/go-sqlite3/callback.go",
        "github.com/mattn/go-sqlite3/error.go",
        "github.com/mattn/go-sqlite3/sqlite3.go",
        "github.com/mattn/go-sqlite3/sqlite3-binding.c",
        "github.com/mattn/go-sqlite3/sqlite3-binding.h",
        "github.com/mattn/go-sqlite3/sqlite3_load_extension.go",
        "github.com/mattn/go-sqlite3/sqlite3_other.go",
        "github.com/mattn/go-sqlite3/sqlite3ext.h",
    ],
    clinkopts = [
        "-lz",
        "-lm",
        "-lpthread",
        "-ldl",
    ],
    copts = [
        "-I",
        "vendor/github.com/mattn/go-sqlite3",
    ],
    tags = ["automanaged"],
)

go_library(
    name = "github.com/mattn/go-sqlite3",
    srcs = ["github.com/mattn/go-sqlite3/doc.go"],
    library = "github.com/mattn/go-sqlite3_cgo",
    tags = ["automanaged"],
)

go_library(
    name = "github.com/satori/go.uuid",
    srcs = ["github.com/satori/go.uuid/uuid.go"],
    tags = ["automanaged"],
)

go_library(
    name = "github.com/spf13/cobra",
    srcs = [
        "github.com/spf13/cobra/bash_completions.go",
        "github.com/spf13/cobra/cobra.go",
        "github.com/spf13/cobra/command.go",
        "github.com/spf13/cobra/command_notwin.go",
    ],
    tags = ["automanaged"],
    deps = ["//vendor:github.com/spf13/pflag"],
)

go_library(
    name = "github.com/spf13/pflag",
    srcs = [
        "github.com/spf13/pflag/bool.go",
        "github.com/spf13/pflag/count.go",
        "github.com/spf13/pflag/duration.go",
        "github.com/spf13/pflag/flag.go",
        "github.com/spf13/pflag/float32.go",
        "github.com/spf13/pflag/float64.go",
        "github.com/spf13/pflag/golangflag.go",
        "github.com/spf13/pflag/int.go",
        "github.com/spf13/pflag/int32.go",
        "github.com/spf13/pflag/int64.go",
        "github.com/spf13/pflag/int8.go",
        "github.com/spf13/pflag/int_slice.go",
        "github.com/spf13/pflag/ip.go",
        "github.com/spf13/pflag/ipmask.go",
        "github.com/spf13/pflag/ipnet.go",
        "github.com/spf13/pflag/string.go",
        "github.com/spf13/pflag/string_slice.go",
        "github.com/spf13/pflag/uint.go",
        "github.com/spf13/pflag/uint16.go",
        "github.com/spf13/pflag/uint32.go",
        "github.com/spf13/pflag/uint64.go",
        "github.com/spf13/pflag/uint8.go",
    ],
    tags = ["automanaged"],
)

go_library(
    name = "golang.org/x/net/context",
    srcs = ["golang.org/x/net/context/context.go"],
    tags = ["automanaged"],
)

go_library(
    name = "golang.org/x/oauth2",
    srcs = [
        "golang.org/x/oauth2/oauth2.go",
        "golang.org/x/oauth2/token.go",
        "golang.org/x/oauth2/transport.go",
    ],
    tags = ["automanaged"],
    deps = [
        "//vendor:golang.org/x/net/context",
        "//vendor:golang.org/x/oauth2/internal",
    ],
)

go_library(
    name = "golang.org/x/oauth2/internal",
    srcs = [
        "golang.org/x/oauth2/internal/oauth2.go",
        "golang.org/x/oauth2/internal/token.go",
        "golang.org/x/oauth2/internal/transport.go",
    ],
    tags = ["automanaged"],
    deps = ["//vendor:golang.org/x/net/context"],
)

go_library(
    name = "golang.org/x/sys/unix",
    srcs = [
        "golang.org/x/sys/unix/asm.s",
        "golang.org/x/sys/unix/asm_linux_amd64.s",
        "golang.org/x/sys/unix/bluetooth_linux.go",
        "golang.org/x/sys/unix/constants.go",
        "golang.org/x/sys/unix/env_unix.go",
        "golang.org/x/sys/unix/env_unset.go",
        "golang.org/x/sys/unix/flock.go",
        "golang.org/x/sys/unix/race0.go",
        "golang.org/x/sys/unix/sockcmsg_linux.go",
        "golang.org/x/sys/unix/sockcmsg_unix.go",
        "golang.org/x/sys/unix/str.go",
        "golang.org/x/sys/unix/syscall.go",
        "golang.org/x/sys/unix/syscall_linux.go",
        "golang.org/x/sys/unix/syscall_linux_amd64.go",
        "golang.org/x/sys/unix/syscall_unix.go",
        "golang.org/x/sys/unix/zerrors_linux_amd64.go",
        "golang.org/x/sys/unix/zsyscall_linux_amd64.go",
        "golang.org/x/sys/unix/zsysnum_linux_amd64.go",
        "golang.org/x/sys/unix/ztypes_linux_amd64.go",
    ],
    tags = ["automanaged"],
)

go_library(
    name = "gopkg.in/yaml.v2",
    srcs = [
        "gopkg.in/yaml.v2/apic.go",
        "gopkg.in/yaml.v2/decode.go",
        "gopkg.in/yaml.v2/emitterc.go",
        "gopkg.in/yaml.v2/encode.go",
        "gopkg.in/yaml.v2/parserc.go",
        "gopkg.in/yaml.v2/readerc.go",
        "gopkg.in/yaml.v2/resolve.go",
        "gopkg.in/yaml.v2/scannerc.go",
        "gopkg.in/yaml.v2/sorter.go",
        "gopkg.in/yaml.v2/writerc.go",
        "gopkg.in/yaml.v2/yaml.go",
        "gopkg.in/yaml.v2/yamlh.go",
        "gopkg.in/yaml.v2/yamlprivateh.go",
    ],
    tags = ["automanaged"],
)
