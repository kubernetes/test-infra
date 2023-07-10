// Please read https://git.k8s.io/test-infra/docs/dep.md before updating dependencies.

module k8s.io/test-infra

replace github.com/golang/lint => golang.org/x/lint v0.0.0-20190301231843-5614ed5bae6f

replace (
	github.com/Azure/azure-sdk-for-go => github.com/Azure/azure-sdk-for-go v63.3.0+incompatible
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v14.2.0+incompatible
	// Upstream is unmaintained. This fork introduces two important changes:
	// * We log an error if writing a cache key fails (e.G. because disk is full)
	// * We inject a header that allows ghproxy to detect if the response was revalidated or a cache miss
	github.com/gregjones/httpcache => github.com/alvaroaleman/httpcache v0.0.0-20210618195546-ab9a1a3f8a38

	golang.org/x/lint => golang.org/x/lint v0.0.0-20190409202823-959b441ac422
	gopkg.in/yaml.v3 => gopkg.in/yaml.v3 v3.0.1
)

require (
	bitbucket.org/creachadair/stringset v0.0.9
	cloud.google.com/go/automl v1.12.0
	cloud.google.com/go/cloudbuild v1.9.0
	cloud.google.com/go/pubsub v1.30.0
	cloud.google.com/go/secretmanager v1.10.0
	cloud.google.com/go/storage v1.28.1
	github.com/Azure/azure-sdk-for-go v68.0.0+incompatible
	github.com/Azure/azure-storage-blob-go v0.8.0
	github.com/Azure/go-autorest/autorest v0.11.29
	github.com/Azure/go-autorest/autorest/adal v0.9.22
	github.com/GoogleCloudPlatform/testgrid v0.0.123
	github.com/NYTimes/gziphandler v1.1.1
	github.com/andygrunwald/go-gerrit v0.0.0-20210709065208-9d38b0be0268
	github.com/andygrunwald/go-jira v1.14.0
	github.com/aws/aws-sdk-go v1.38.49
	github.com/bazelbuild/buildtools v0.0.0-20200922170545-10384511ce98
	github.com/blang/semver/v4 v4.0.0
	github.com/bwmarrin/snowflake v0.0.0
	github.com/clarketm/json v1.13.4
	github.com/client9/misspell v0.3.4
	github.com/denormal/go-gitignore v0.0.0-20180930084346-ae8ad1d07817
	github.com/dgrijalva/jwt-go/v4 v4.0.0-preview1
	github.com/djherbis/atime v1.0.0
	github.com/evanphx/json-patch v4.12.0+incompatible
	github.com/felixge/fgprof v0.9.1
	github.com/fsnotify/fsnotify v1.5.1
	github.com/fsouza/fake-gcs-server v1.19.4
	github.com/go-bindata/go-bindata/v3 v3.1.3
	github.com/go-git/go-git/v5 v5.6.1
	github.com/go-openapi/spec v0.20.4
	github.com/go-test/deep v1.0.7
	github.com/golang/glog v1.1.0
	github.com/gomodule/redigo v1.8.5
	github.com/google/go-cmp v0.5.9
	github.com/google/go-github v17.0.0+incompatible
	github.com/google/gofuzz v1.2.1-0.20210504230335-f78f29fc09ea
	github.com/google/uuid v1.3.0
	github.com/gorilla/csrf v1.6.2
	github.com/gorilla/mux v1.8.0
	github.com/gorilla/securecookie v1.1.1
	github.com/gorilla/sessions v1.2.0
	github.com/gregjones/httpcache v0.0.0-20190212212710-3befbb6ad0cc
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0
	github.com/hashicorp/go-multierror v1.1.1
	github.com/hashicorp/go-retryablehttp v0.7.2
	github.com/hashicorp/golang-lru v0.5.4
	github.com/klauspost/pgzip v1.2.1
	github.com/mattn/go-zglob v0.0.2
	github.com/maxbrunsfeld/counterfeiter/v6 v6.4.1
	github.com/pelletier/go-toml v1.9.3
	github.com/peterbourgon/diskv v2.0.1+incompatible
	github.com/prometheus/client_golang v1.13.0
	github.com/prometheus/client_model v0.3.0
	github.com/prometheus/common v0.37.0
	github.com/shurcooL/githubv4 v0.0.0-20210725200734-83ba7b4c9228
	github.com/sirupsen/logrus v1.9.0
	github.com/spf13/cobra v1.7.0
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.8.3
	github.com/tektoncd/pipeline v0.45.0
	go.uber.org/zap v1.24.0
	go4.org v0.0.0-20201209231011-d4a079459e60
	gocloud.dev v0.19.0
	golang.org/x/crypto v0.9.0
	golang.org/x/lint v0.0.0-20210508222113-6edffad5e616
	golang.org/x/net v0.10.0
	golang.org/x/oauth2 v0.8.0
	golang.org/x/sync v0.2.0
	golang.org/x/text v0.9.0
	golang.org/x/time v0.3.0
	golang.org/x/tools v0.8.0
	gomodules.xyz/jsonpatch/v2 v2.2.0
	google.golang.org/api v0.121.0
	google.golang.org/genproto v0.0.0-20230410155749-daa745c078e1
	google.golang.org/grpc v1.55.0
	google.golang.org/protobuf v1.30.0
	gopkg.in/fsnotify.v1 v1.4.7
	gopkg.in/ini.v1 v1.62.0
	gopkg.in/robfig/cron.v2 v2.0.0-20150107220207-be2e0b0deed5
	gopkg.in/yaml.v2 v2.4.0
	gopkg.in/yaml.v3 v3.0.1
	k8s.io/api v0.25.9
	k8s.io/apimachinery v0.26.5
	k8s.io/client-go v0.25.9
	k8s.io/code-generator v0.25.9
	k8s.io/klog/v2 v2.90.1
	k8s.io/utils v0.0.0-20230209194617-a36077c30491
	knative.dev/pkg v0.0.0-20230221145627-8efb3485adcf
	sigs.k8s.io/controller-runtime v0.12.3
	sigs.k8s.io/controller-tools v0.9.2
	sigs.k8s.io/yaml v1.3.0
)

require (
	cloud.google.com/go v0.110.0 // indirect
	cloud.google.com/go/compute v1.19.1 // indirect
	cloud.google.com/go/compute/metadata v0.2.3 // indirect
	cloud.google.com/go/iam v0.13.0 // indirect
	cloud.google.com/go/longrunning v0.4.1 // indirect
	contrib.go.opencensus.io/exporter/ocagent v0.7.1-0.20200907061046-05415f1de66d // indirect
	contrib.go.opencensus.io/exporter/prometheus v0.4.0 // indirect
	github.com/Azure/azure-pipeline-go v0.2.2 // indirect
	github.com/Azure/go-autorest v14.2.0+incompatible // indirect
	github.com/Azure/go-autorest/autorest/date v0.3.0 // indirect
	github.com/Azure/go-autorest/autorest/to v0.4.0 // indirect
	github.com/Azure/go-autorest/autorest/validation v0.3.1 // indirect
	github.com/Azure/go-autorest/logger v0.2.1 // indirect
	github.com/Azure/go-autorest/tracing v0.6.0 // indirect
	github.com/Microsoft/go-winio v0.6.1 // indirect
	github.com/ProtonMail/go-crypto v0.0.0-20230217124315-7d5c6f04bbb8 // indirect
	github.com/acomagu/bufpipe v1.0.4 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/blendle/zapdriver v1.3.1 // indirect
	github.com/census-instrumentation/opencensus-proto v0.4.1 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/cloudflare/circl v1.1.0 // indirect
	github.com/danwakefield/fnmatch v0.0.0-20160403171240-cbb64ac3d964 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/emicklei/go-restful/v3 v3.9.0 // indirect
	github.com/emirpasic/gods v1.18.1 // indirect
	github.com/evanphx/json-patch/v5 v5.6.0 // indirect
	github.com/fatih/color v1.13.0 // indirect
	github.com/fatih/structs v1.1.0 // indirect
	github.com/fvbommel/sortorder v1.0.1 // indirect
	github.com/go-git/gcfg v1.5.0 // indirect
	github.com/go-git/go-billy/v5 v5.4.1 // indirect
	github.com/go-kit/log v0.2.0 // indirect
	github.com/go-logfmt/logfmt v0.5.1 // indirect
	github.com/go-logr/logr v1.2.4 // indirect
	github.com/go-logr/zapr v1.2.3 // indirect
	github.com/go-openapi/jsonpointer v0.19.6 // indirect
	github.com/go-openapi/jsonreference v0.20.1 // indirect
	github.com/go-openapi/swag v0.22.3 // indirect
	github.com/gobuffalo/flect v0.2.5 // indirect
	github.com/gofrs/uuid v4.2.0+incompatible // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang-jwt/jwt v3.2.1+incompatible // indirect
	github.com/golang-jwt/jwt/v4 v4.5.0 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/google/btree v1.0.1 // indirect
	github.com/google/gnostic v0.6.9 // indirect
	github.com/google/go-containerregistry v0.15.2 // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/google/pprof v0.0.0-20210720184732-4bb14d4b1be1 // indirect
	github.com/google/s2a-go v0.1.3 // indirect
	github.com/google/wire v0.4.0 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.2.3 // indirect
	github.com/googleapis/gax-go v2.0.2+incompatible // indirect
	github.com/googleapis/gax-go/v2 v2.8.0 // indirect
	github.com/gorilla/handlers v1.4.2 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.11.3 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/imdario/mergo v0.3.13 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jbenet/go-context v0.0.0-20150711004518-d14ea06fba99 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/kevinburke/ssh_config v1.2.0 // indirect
	github.com/kisielk/errcheck v1.5.0 // indirect
	github.com/klauspost/compress v1.16.5 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-ieproxy v0.0.1 // indirect
	github.com/mattn/go-isatty v0.0.16 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/pjbgf/sha1cd v0.3.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/procfs v0.8.0 // indirect
	github.com/prometheus/statsd_exporter v0.21.0 // indirect
	github.com/sergi/go-diff v1.2.0 // indirect
	github.com/shurcooL/graphql v0.0.0-20181231061246-d48a9a75455f // indirect
	github.com/skeema/knownhosts v1.1.0 // indirect
	github.com/smartystreets/goconvey v1.8.1 // indirect
	github.com/trivago/tgo v1.0.7 // indirect
	github.com/xanzy/ssh-agent v0.3.3 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.uber.org/atomic v1.10.0 // indirect
	go.uber.org/multierr v1.8.0 // indirect
	golang.org/x/mod v0.10.0 // indirect
	golang.org/x/sys v0.8.0 // indirect
	golang.org/x/term v0.8.0 // indirect
	golang.org/x/xerrors v0.0.0-20220907171357-04be3eba64a2 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/warnings.v0 v0.1.2 // indirect
	k8s.io/apiextensions-apiserver v0.25.4 // indirect
	k8s.io/component-base v0.25.4 // indirect
	k8s.io/gengo v0.0.0-20221011193443-fad74ee6edd9 // indirect
	k8s.io/kube-openapi v0.0.0-20230308215209-15aac26d736a // indirect
	sigs.k8s.io/json v0.0.0-20221116044647-bc3834ca7abd // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.2.3 // indirect
)

go 1.18
