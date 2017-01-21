package(default_visibility = ["//visibility:public"])

licenses(["notice"])

load(
    "@io_bazel_rules_go//go:def.bzl",
    "cgo_genrule",
    "go_library",
)

go_library(
    name = "bitbucket.org/ww/goautoneg",
    srcs = ["bitbucket.org/ww/goautoneg/autoneg.go"],
    tags = ["automanaged"],
)

go_library(
    name = "cloud.google.com/go/compute/metadata",
    srcs = ["cloud.google.com/go/compute/metadata/metadata.go"],
    tags = ["automanaged"],
    deps = [
        "//vendor:cloud.google.com/go/internal",
        "//vendor:golang.org/x/net/context",
        "//vendor:golang.org/x/net/context/ctxhttp",
    ],
)

go_library(
    name = "cloud.google.com/go/internal",
    srcs = ["cloud.google.com/go/internal/cloud.go"],
    tags = ["automanaged"],
)

go_library(
    name = "cloud.google.com/go/pubsub",
    srcs = [
        "cloud.google.com/go/pubsub/acker.go",
        "cloud.google.com/go/pubsub/doc.go",
        "cloud.google.com/go/pubsub/iterator.go",
        "cloud.google.com/go/pubsub/keepalive.go",
        "cloud.google.com/go/pubsub/message.go",
        "cloud.google.com/go/pubsub/pubsub.go",
        "cloud.google.com/go/pubsub/puller.go",
        "cloud.google.com/go/pubsub/service.go",
        "cloud.google.com/go/pubsub/subscription.go",
        "cloud.google.com/go/pubsub/topic.go",
    ],
    tags = ["automanaged"],
    deps = [
        "//vendor:golang.org/x/net/context",
        "//vendor:google.golang.org/api/googleapi",
        "//vendor:google.golang.org/api/iterator",
        "//vendor:google.golang.org/api/option",
        "//vendor:google.golang.org/api/pubsub/v1",
        "//vendor:google.golang.org/api/transport",
    ],
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
    name = "github.com/arbovm/levenshtein",
    srcs = ["github.com/arbovm/levenshtein/levenshtein.go"],
    tags = ["automanaged"],
)

go_library(
    name = "github.com/beorn7/perks/quantile",
    srcs = ["github.com/beorn7/perks/quantile/stream.go"],
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
    name = "github.com/google/btree",
    srcs = ["github.com/google/btree/btree.go"],
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
        "github.com/google/go-github/github/repos_traffic.go",
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
    name = "github.com/google/gofuzz",
    srcs = [
        "github.com/google/gofuzz/doc.go",
        "github.com/google/gofuzz/fuzz.go",
    ],
    tags = ["automanaged"],
)

go_library(
    name = "github.com/gregjones/httpcache",
    srcs = ["github.com/gregjones/httpcache/httpcache.go"],
    tags = ["automanaged"],
)

go_library(
    name = "github.com/gregjones/httpcache/diskcache",
    srcs = ["github.com/gregjones/httpcache/diskcache/diskcache.go"],
    tags = ["automanaged"],
    deps = ["//vendor:github.com/peterbourgon/diskv"],
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
        "-O",
    ],
    tags = ["automanaged"],
)

go_library(
    name = "github.com/mattn/go-sqlite3",
    srcs = ["github.com/mattn/go-sqlite3/doc.go"],
    library = ":github.com/mattn/go-sqlite3_cgo",
    tags = ["automanaged"],
)

go_library(
    name = "github.com/matttproud/golang_protobuf_extensions/pbutil",
    srcs = [
        "github.com/matttproud/golang_protobuf_extensions/pbutil/decode.go",
        "github.com/matttproud/golang_protobuf_extensions/pbutil/doc.go",
        "github.com/matttproud/golang_protobuf_extensions/pbutil/encode.go",
    ],
    tags = ["automanaged"],
    deps = ["//vendor:github.com/golang/protobuf/proto"],
)

go_library(
    name = "github.com/peterbourgon/diskv",
    srcs = [
        "github.com/peterbourgon/diskv/compression.go",
        "github.com/peterbourgon/diskv/diskv.go",
        "github.com/peterbourgon/diskv/index.go",
    ],
    tags = ["automanaged"],
    deps = ["//vendor:github.com/google/btree"],
)

go_library(
    name = "github.com/prometheus/client_golang/prometheus",
    srcs = [
        "github.com/prometheus/client_golang/prometheus/collector.go",
        "github.com/prometheus/client_golang/prometheus/counter.go",
        "github.com/prometheus/client_golang/prometheus/desc.go",
        "github.com/prometheus/client_golang/prometheus/doc.go",
        "github.com/prometheus/client_golang/prometheus/expvar_collector.go",
        "github.com/prometheus/client_golang/prometheus/fnv.go",
        "github.com/prometheus/client_golang/prometheus/gauge.go",
        "github.com/prometheus/client_golang/prometheus/go_collector.go",
        "github.com/prometheus/client_golang/prometheus/histogram.go",
        "github.com/prometheus/client_golang/prometheus/http.go",
        "github.com/prometheus/client_golang/prometheus/metric.go",
        "github.com/prometheus/client_golang/prometheus/process_collector.go",
        "github.com/prometheus/client_golang/prometheus/registry.go",
        "github.com/prometheus/client_golang/prometheus/summary.go",
        "github.com/prometheus/client_golang/prometheus/untyped.go",
        "github.com/prometheus/client_golang/prometheus/value.go",
        "github.com/prometheus/client_golang/prometheus/vec.go",
    ],
    tags = ["automanaged"],
    deps = [
        "//vendor:github.com/beorn7/perks/quantile",
        "//vendor:github.com/golang/protobuf/proto",
        "//vendor:github.com/prometheus/client_model/go",
        "//vendor:github.com/prometheus/common/expfmt",
        "//vendor:github.com/prometheus/common/model",
        "//vendor:github.com/prometheus/procfs",
    ],
)

go_library(
    name = "github.com/prometheus/client_golang/prometheus/promhttp",
    srcs = ["github.com/prometheus/client_golang/prometheus/promhttp/http.go"],
    tags = ["automanaged"],
    deps = [
        "//vendor:github.com/prometheus/client_golang/prometheus",
        "//vendor:github.com/prometheus/common/expfmt",
    ],
)

go_library(
    name = "github.com/prometheus/client_model/go",
    srcs = ["github.com/prometheus/client_model/go/metrics.pb.go"],
    tags = ["automanaged"],
    deps = ["//vendor:github.com/golang/protobuf/proto"],
)

go_library(
    name = "github.com/prometheus/common/expfmt",
    srcs = [
        "github.com/prometheus/common/expfmt/decode.go",
        "github.com/prometheus/common/expfmt/encode.go",
        "github.com/prometheus/common/expfmt/expfmt.go",
        "github.com/prometheus/common/expfmt/json_decode.go",
        "github.com/prometheus/common/expfmt/text_create.go",
        "github.com/prometheus/common/expfmt/text_parse.go",
    ],
    tags = ["automanaged"],
    deps = [
        "//vendor:bitbucket.org/ww/goautoneg",
        "//vendor:github.com/golang/protobuf/proto",
        "//vendor:github.com/matttproud/golang_protobuf_extensions/pbutil",
        "//vendor:github.com/prometheus/client_model/go",
        "//vendor:github.com/prometheus/common/model",
    ],
)

go_library(
    name = "github.com/prometheus/common/model",
    srcs = [
        "github.com/prometheus/common/model/alert.go",
        "github.com/prometheus/common/model/fingerprinting.go",
        "github.com/prometheus/common/model/labels.go",
        "github.com/prometheus/common/model/labelset.go",
        "github.com/prometheus/common/model/metric.go",
        "github.com/prometheus/common/model/model.go",
        "github.com/prometheus/common/model/signature.go",
        "github.com/prometheus/common/model/silence.go",
        "github.com/prometheus/common/model/time.go",
        "github.com/prometheus/common/model/value.go",
    ],
    tags = ["automanaged"],
)

go_library(
    name = "github.com/prometheus/procfs",
    srcs = [
        "github.com/prometheus/procfs/doc.go",
        "github.com/prometheus/procfs/fs.go",
        "github.com/prometheus/procfs/ipvs.go",
        "github.com/prometheus/procfs/mdstat.go",
        "github.com/prometheus/procfs/proc.go",
        "github.com/prometheus/procfs/proc_io.go",
        "github.com/prometheus/procfs/proc_limits.go",
        "github.com/prometheus/procfs/proc_stat.go",
        "github.com/prometheus/procfs/stat.go",
    ],
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
    srcs = [
        "golang.org/x/net/context/context.go",
        "golang.org/x/net/context/go17.go",
    ],
    tags = ["automanaged"],
)

go_library(
    name = "golang.org/x/net/context/ctxhttp",
    srcs = ["golang.org/x/net/context/ctxhttp/ctxhttp.go"],
    tags = ["automanaged"],
    deps = ["//vendor:golang.org/x/net/context"],
)

go_library(
    name = "golang.org/x/net/http2",
    srcs = [
        "golang.org/x/net/http2/client_conn_pool.go",
        "golang.org/x/net/http2/configure_transport.go",
        "golang.org/x/net/http2/errors.go",
        "golang.org/x/net/http2/fixed_buffer.go",
        "golang.org/x/net/http2/flow.go",
        "golang.org/x/net/http2/frame.go",
        "golang.org/x/net/http2/go16.go",
        "golang.org/x/net/http2/go17.go",
        "golang.org/x/net/http2/go17_not18.go",
        "golang.org/x/net/http2/gotrack.go",
        "golang.org/x/net/http2/headermap.go",
        "golang.org/x/net/http2/http2.go",
        "golang.org/x/net/http2/not_go18.go",
        "golang.org/x/net/http2/pipe.go",
        "golang.org/x/net/http2/server.go",
        "golang.org/x/net/http2/transport.go",
        "golang.org/x/net/http2/write.go",
        "golang.org/x/net/http2/writesched.go",
        "golang.org/x/net/http2/writesched_priority.go",
        "golang.org/x/net/http2/writesched_random.go",
    ],
    tags = ["automanaged"],
    deps = [
        "//vendor:golang.org/x/net/http2/hpack",
        "//vendor:golang.org/x/net/idna",
        "//vendor:golang.org/x/net/lex/httplex",
    ],
)

go_library(
    name = "golang.org/x/net/http2/hpack",
    srcs = [
        "golang.org/x/net/http2/hpack/encode.go",
        "golang.org/x/net/http2/hpack/hpack.go",
        "golang.org/x/net/http2/hpack/huffman.go",
        "golang.org/x/net/http2/hpack/tables.go",
    ],
    tags = ["automanaged"],
)

go_library(
    name = "golang.org/x/net/idna",
    srcs = [
        "golang.org/x/net/idna/idna.go",
        "golang.org/x/net/idna/punycode.go",
    ],
    tags = ["automanaged"],
)

go_library(
    name = "golang.org/x/net/internal/timeseries",
    srcs = ["golang.org/x/net/internal/timeseries/timeseries.go"],
    tags = ["automanaged"],
)

go_library(
    name = "golang.org/x/net/lex/httplex",
    srcs = ["golang.org/x/net/lex/httplex/httplex.go"],
    tags = ["automanaged"],
    deps = ["//vendor:golang.org/x/net/idna"],
)

go_library(
    name = "golang.org/x/net/trace",
    srcs = [
        "golang.org/x/net/trace/events.go",
        "golang.org/x/net/trace/histogram.go",
        "golang.org/x/net/trace/trace.go",
    ],
    tags = ["automanaged"],
    deps = [
        "//vendor:golang.org/x/net/context",
        "//vendor:golang.org/x/net/internal/timeseries",
    ],
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
    name = "golang.org/x/oauth2/google",
    srcs = [
        "golang.org/x/oauth2/google/appengine.go",
        "golang.org/x/oauth2/google/default.go",
        "golang.org/x/oauth2/google/google.go",
        "golang.org/x/oauth2/google/jwt.go",
        "golang.org/x/oauth2/google/sdk.go",
    ],
    tags = ["automanaged"],
    deps = [
        "//vendor:cloud.google.com/go/compute/metadata",
        "//vendor:golang.org/x/net/context",
        "//vendor:golang.org/x/oauth2",
        "//vendor:golang.org/x/oauth2/internal",
        "//vendor:golang.org/x/oauth2/jws",
        "//vendor:golang.org/x/oauth2/jwt",
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
    name = "golang.org/x/oauth2/jws",
    srcs = ["golang.org/x/oauth2/jws/jws.go"],
    tags = ["automanaged"],
)

go_library(
    name = "golang.org/x/oauth2/jwt",
    srcs = ["golang.org/x/oauth2/jwt/jwt.go"],
    tags = ["automanaged"],
    deps = [
        "//vendor:golang.org/x/net/context",
        "//vendor:golang.org/x/oauth2",
        "//vendor:golang.org/x/oauth2/internal",
        "//vendor:golang.org/x/oauth2/jws",
    ],
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
    name = "google.golang.org/api/gensupport",
    srcs = [
        "google.golang.org/api/gensupport/backoff.go",
        "google.golang.org/api/gensupport/buffer.go",
        "google.golang.org/api/gensupport/doc.go",
        "google.golang.org/api/gensupport/json.go",
        "google.golang.org/api/gensupport/media.go",
        "google.golang.org/api/gensupport/params.go",
        "google.golang.org/api/gensupport/resumable.go",
        "google.golang.org/api/gensupport/retry.go",
        "google.golang.org/api/gensupport/send.go",
    ],
    tags = ["automanaged"],
    deps = [
        "//vendor:golang.org/x/net/context",
        "//vendor:golang.org/x/net/context/ctxhttp",
        "//vendor:google.golang.org/api/googleapi",
    ],
)

go_library(
    name = "google.golang.org/api/googleapi",
    srcs = [
        "google.golang.org/api/googleapi/googleapi.go",
        "google.golang.org/api/googleapi/types.go",
    ],
    tags = ["automanaged"],
    deps = ["//vendor:google.golang.org/api/googleapi/internal/uritemplates"],
)

go_library(
    name = "google.golang.org/api/googleapi/internal/uritemplates",
    srcs = [
        "google.golang.org/api/googleapi/internal/uritemplates/uritemplates.go",
        "google.golang.org/api/googleapi/internal/uritemplates/utils.go",
    ],
    tags = ["automanaged"],
)

go_library(
    name = "google.golang.org/api/internal",
    srcs = [
        "google.golang.org/api/internal/pool.go",
        "google.golang.org/api/internal/settings.go",
    ],
    tags = ["automanaged"],
    deps = [
        "//vendor:golang.org/x/oauth2",
        "//vendor:google.golang.org/grpc",
        "//vendor:google.golang.org/grpc/naming",
    ],
)

go_library(
    name = "google.golang.org/api/iterator",
    srcs = ["google.golang.org/api/iterator/iterator.go"],
    tags = ["automanaged"],
)

go_library(
    name = "google.golang.org/api/option",
    srcs = ["google.golang.org/api/option/option.go"],
    tags = ["automanaged"],
    deps = [
        "//vendor:golang.org/x/oauth2",
        "//vendor:google.golang.org/api/internal",
        "//vendor:google.golang.org/grpc",
    ],
)

go_library(
    name = "google.golang.org/api/pubsub/v1",
    srcs = ["google.golang.org/api/pubsub/v1/pubsub-gen.go"],
    tags = ["automanaged"],
    deps = [
        "//vendor:golang.org/x/net/context",
        "//vendor:golang.org/x/net/context/ctxhttp",
        "//vendor:google.golang.org/api/gensupport",
        "//vendor:google.golang.org/api/googleapi",
    ],
)

go_library(
    name = "google.golang.org/api/transport",
    srcs = ["google.golang.org/api/transport/dial.go"],
    tags = ["automanaged"],
    deps = [
        "//vendor:golang.org/x/net/context",
        "//vendor:golang.org/x/oauth2",
        "//vendor:golang.org/x/oauth2/google",
        "//vendor:google.golang.org/api/internal",
        "//vendor:google.golang.org/api/option",
        "//vendor:google.golang.org/grpc",
        "//vendor:google.golang.org/grpc/credentials",
        "//vendor:google.golang.org/grpc/credentials/oauth",
    ],
)

go_library(
    name = "google.golang.org/appengine",
    srcs = [
        "google.golang.org/appengine/appengine.go",
        "google.golang.org/appengine/appengine_vm.go",
        "google.golang.org/appengine/errors.go",
        "google.golang.org/appengine/identity.go",
        "google.golang.org/appengine/namespace.go",
        "google.golang.org/appengine/timeout.go",
    ],
    tags = ["automanaged"],
    deps = [
        "//vendor:github.com/golang/protobuf/proto",
        "//vendor:golang.org/x/net/context",
        "//vendor:google.golang.org/appengine/internal",
        "//vendor:google.golang.org/appengine/internal/app_identity",
        "//vendor:google.golang.org/appengine/internal/modules",
    ],
)

go_library(
    name = "google.golang.org/appengine/internal",
    srcs = [
        "google.golang.org/appengine/internal/api.go",
        "google.golang.org/appengine/internal/api_common.go",
        "google.golang.org/appengine/internal/app_id.go",
        "google.golang.org/appengine/internal/identity.go",
        "google.golang.org/appengine/internal/identity_vm.go",
        "google.golang.org/appengine/internal/internal.go",
        "google.golang.org/appengine/internal/main_vm.go",
        "google.golang.org/appengine/internal/metadata.go",
        "google.golang.org/appengine/internal/net.go",
        "google.golang.org/appengine/internal/transaction.go",
    ],
    tags = ["automanaged"],
    deps = [
        "//vendor:github.com/golang/protobuf/proto",
        "//vendor:golang.org/x/net/context",
        "//vendor:google.golang.org/appengine/internal/base",
        "//vendor:google.golang.org/appengine/internal/datastore",
        "//vendor:google.golang.org/appengine/internal/log",
        "//vendor:google.golang.org/appengine/internal/remote_api",
    ],
)

go_library(
    name = "google.golang.org/appengine/internal/app_identity",
    srcs = ["google.golang.org/appengine/internal/app_identity/app_identity_service.pb.go"],
    tags = ["automanaged"],
    deps = ["//vendor:github.com/golang/protobuf/proto"],
)

go_library(
    name = "google.golang.org/appengine/internal/base",
    srcs = ["google.golang.org/appengine/internal/base/api_base.pb.go"],
    tags = ["automanaged"],
    deps = ["//vendor:github.com/golang/protobuf/proto"],
)

go_library(
    name = "google.golang.org/appengine/internal/datastore",
    srcs = ["google.golang.org/appengine/internal/datastore/datastore_v3.pb.go"],
    tags = ["automanaged"],
    deps = ["//vendor:github.com/golang/protobuf/proto"],
)

go_library(
    name = "google.golang.org/appengine/internal/log",
    srcs = ["google.golang.org/appengine/internal/log/log_service.pb.go"],
    tags = ["automanaged"],
    deps = ["//vendor:github.com/golang/protobuf/proto"],
)

go_library(
    name = "google.golang.org/appengine/internal/modules",
    srcs = ["google.golang.org/appengine/internal/modules/modules_service.pb.go"],
    tags = ["automanaged"],
    deps = ["//vendor:github.com/golang/protobuf/proto"],
)

go_library(
    name = "google.golang.org/appengine/internal/remote_api",
    srcs = ["google.golang.org/appengine/internal/remote_api/remote_api.pb.go"],
    tags = ["automanaged"],
    deps = ["//vendor:github.com/golang/protobuf/proto"],
)

go_library(
    name = "google.golang.org/grpc",
    srcs = [
        "google.golang.org/grpc/backoff.go",
        "google.golang.org/grpc/balancer.go",
        "google.golang.org/grpc/call.go",
        "google.golang.org/grpc/clientconn.go",
        "google.golang.org/grpc/doc.go",
        "google.golang.org/grpc/interceptor.go",
        "google.golang.org/grpc/rpc_util.go",
        "google.golang.org/grpc/server.go",
        "google.golang.org/grpc/stream.go",
        "google.golang.org/grpc/trace.go",
    ],
    tags = ["automanaged"],
    deps = [
        "//vendor:github.com/golang/protobuf/proto",
        "//vendor:golang.org/x/net/context",
        "//vendor:golang.org/x/net/http2",
        "//vendor:golang.org/x/net/trace",
        "//vendor:google.golang.org/grpc/codes",
        "//vendor:google.golang.org/grpc/credentials",
        "//vendor:google.golang.org/grpc/grpclog",
        "//vendor:google.golang.org/grpc/internal",
        "//vendor:google.golang.org/grpc/metadata",
        "//vendor:google.golang.org/grpc/naming",
        "//vendor:google.golang.org/grpc/transport",
    ],
)

go_library(
    name = "google.golang.org/grpc/codes",
    srcs = [
        "google.golang.org/grpc/codes/code_string.go",
        "google.golang.org/grpc/codes/codes.go",
    ],
    tags = ["automanaged"],
)

go_library(
    name = "google.golang.org/grpc/credentials",
    srcs = [
        "google.golang.org/grpc/credentials/credentials.go",
        "google.golang.org/grpc/credentials/credentials_util_go17.go",
    ],
    tags = ["automanaged"],
    deps = ["//vendor:golang.org/x/net/context"],
)

go_library(
    name = "google.golang.org/grpc/credentials/oauth",
    srcs = ["google.golang.org/grpc/credentials/oauth/oauth.go"],
    tags = ["automanaged"],
    deps = [
        "//vendor:golang.org/x/net/context",
        "//vendor:golang.org/x/oauth2",
        "//vendor:golang.org/x/oauth2/google",
        "//vendor:golang.org/x/oauth2/jwt",
        "//vendor:google.golang.org/grpc/credentials",
    ],
)

go_library(
    name = "google.golang.org/grpc/grpclog",
    srcs = ["google.golang.org/grpc/grpclog/logger.go"],
    tags = ["automanaged"],
)

go_library(
    name = "google.golang.org/grpc/internal",
    srcs = ["google.golang.org/grpc/internal/internal.go"],
    tags = ["automanaged"],
)

go_library(
    name = "google.golang.org/grpc/metadata",
    srcs = ["google.golang.org/grpc/metadata/metadata.go"],
    tags = ["automanaged"],
    deps = ["//vendor:golang.org/x/net/context"],
)

go_library(
    name = "google.golang.org/grpc/naming",
    srcs = ["google.golang.org/grpc/naming/naming.go"],
    tags = ["automanaged"],
)

go_library(
    name = "google.golang.org/grpc/peer",
    srcs = ["google.golang.org/grpc/peer/peer.go"],
    tags = ["automanaged"],
    deps = [
        "//vendor:golang.org/x/net/context",
        "//vendor:google.golang.org/grpc/credentials",
    ],
)

go_library(
    name = "google.golang.org/grpc/transport",
    srcs = [
        "google.golang.org/grpc/transport/control.go",
        "google.golang.org/grpc/transport/go17.go",
        "google.golang.org/grpc/transport/handler_server.go",
        "google.golang.org/grpc/transport/http2_client.go",
        "google.golang.org/grpc/transport/http2_server.go",
        "google.golang.org/grpc/transport/http_util.go",
        "google.golang.org/grpc/transport/transport.go",
    ],
    tags = ["automanaged"],
    deps = [
        "//vendor:golang.org/x/net/context",
        "//vendor:golang.org/x/net/http2",
        "//vendor:golang.org/x/net/http2/hpack",
        "//vendor:golang.org/x/net/trace",
        "//vendor:google.golang.org/grpc/codes",
        "//vendor:google.golang.org/grpc/credentials",
        "//vendor:google.golang.org/grpc/grpclog",
        "//vendor:google.golang.org/grpc/metadata",
        "//vendor:google.golang.org/grpc/peer",
    ],
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

go_library(
    name = "k8s.io/contrib/test-utils/utils",
    srcs = [
        "k8s.io/contrib/test-utils/utils/bucket.go",
        "k8s.io/contrib/test-utils/utils/utils.go",
    ],
    tags = ["automanaged"],
    deps = ["//vendor:github.com/golang/glog"],
)

go_library(
    name = "k8s.io/kubernetes/pkg/util/clock",
    srcs = ["k8s.io/kubernetes/pkg/util/clock/clock.go"],
    tags = ["automanaged"],
)

go_library(
    name = "k8s.io/kubernetes/pkg/util/errors",
    srcs = [
        "k8s.io/kubernetes/pkg/util/errors/doc.go",
        "k8s.io/kubernetes/pkg/util/errors/errors.go",
    ],
    tags = ["automanaged"],
)

go_library(
    name = "k8s.io/kubernetes/pkg/util/flag",
    srcs = [
        "k8s.io/kubernetes/pkg/util/flag/flags.go",
        "k8s.io/kubernetes/pkg/util/flag/tristate.go",
    ],
    tags = ["automanaged"],
    deps = [
        "//vendor:github.com/golang/glog",
        "//vendor:github.com/spf13/pflag",
    ],
)

go_library(
    name = "k8s.io/kubernetes/pkg/util/sets",
    srcs = [
        "k8s.io/kubernetes/pkg/util/sets/byte.go",
        "k8s.io/kubernetes/pkg/util/sets/doc.go",
        "k8s.io/kubernetes/pkg/util/sets/empty.go",
        "k8s.io/kubernetes/pkg/util/sets/int.go",
        "k8s.io/kubernetes/pkg/util/sets/int64.go",
        "k8s.io/kubernetes/pkg/util/sets/string.go",
    ],
    tags = ["automanaged"],
)

go_library(
    name = "k8s.io/kubernetes/pkg/util/yaml",
    srcs = ["k8s.io/kubernetes/pkg/util/yaml/decoder.go"],
    tags = ["automanaged"],
    deps = [
        "//vendor:github.com/ghodss/yaml",
        "//vendor:github.com/golang/glog",
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
    srcs = [":package-srcs"],
    tags = ["automanaged"],
)
