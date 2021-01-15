// Please read https://git.k8s.io/test-infra/docs/dep.md before updating dependencies.

module k8s.io/test-infra

replace github.com/tektoncd/pipeline => github.com/howardjohn/pipeline-1 v0.19.1-0.20210108211745-9e049aeea460

require (
	cloud.google.com/go/pubsub v1.4.0
	cloud.google.com/go/storage v1.12.0
	github.com/GoogleCloudPlatform/testgrid v0.0.30
	github.com/NYTimes/gziphandler v0.0.0-20170623195520-56545f4a5d46
	github.com/andygrunwald/go-gerrit v0.0.0-20190120104749-174420ebee6c
	github.com/andygrunwald/go-jira v1.13.0
	github.com/aws/aws-sdk-go v1.31.12
	github.com/bazelbuild/buildtools v0.0.0-20190917191645-69366ca98f89
	github.com/blang/semver v3.5.1+incompatible
	github.com/bwmarrin/snowflake v0.0.0
	github.com/clarketm/json v1.13.4
	github.com/client9/misspell v0.3.4
	github.com/dgrijalva/jwt-go/v4 v4.0.0-preview1
	github.com/djherbis/atime v1.0.0
	github.com/evanphx/json-patch v4.9.0+incompatible
	github.com/fsnotify/fsnotify v1.4.9
	github.com/fsouza/fake-gcs-server v1.19.4
	github.com/go-bindata/go-bindata/v3 v3.1.3
	github.com/go-openapi/spec v0.19.6
	github.com/go-test/deep v1.0.4
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/golang/mock v1.4.4
	github.com/gomodule/redigo v1.7.0
	github.com/google/go-cmp v0.5.4
	github.com/google/go-github v17.0.0+incompatible
	github.com/google/gofuzz v1.1.0
	github.com/google/uuid v1.1.2
	github.com/gorilla/csrf v1.6.2
	github.com/gorilla/mux v1.7.4
	github.com/gorilla/securecookie v1.1.1
	github.com/gorilla/sessions v1.2.0
	github.com/gregjones/httpcache v0.0.0-20190212212710-3befbb6ad0cc
	github.com/hashicorp/go-retryablehttp v0.6.6
	github.com/influxdata/influxdb v0.0.0-20161215172503-049f9b42e9a5
	github.com/jinzhu/gorm v1.9.12
	github.com/klauspost/compress v1.11.6 // indirect
	github.com/klauspost/pgzip v1.2.1
	github.com/mattn/go-zglob v0.0.2
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826
	github.com/peterbourgon/diskv v2.0.1+incompatible
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.8.0
	github.com/prometheus/client_model v0.2.0
	github.com/prometheus/common v0.15.0
	github.com/satori/go.uuid v1.2.0
	github.com/shurcooL/githubv4 v0.0.0-20191102174205-af46314aec7b
	github.com/sirupsen/logrus v1.6.0
	github.com/spf13/cobra v1.0.0
	github.com/spf13/pflag v1.0.5
	github.com/tektoncd/pipeline v0.13.1-0.20200625065359-44f22a067b75
	go.uber.org/zap v1.16.0
	gocloud.dev v0.19.0
	golang.org/x/lint v0.0.0-20200302205851-738671d3881b
	golang.org/x/net v0.0.0-20201209123823-ac852fbbde11
	golang.org/x/oauth2 v0.0.0-20201208152858-08078c50e5b5
	golang.org/x/sync v0.0.0-20201207232520-09787c993a3a
	golang.org/x/time v0.0.0-20200630173020-3af7569d3a1e
	golang.org/x/tools v0.0.0-20201211185031-d93e913c1a58
	google.golang.org/api v0.36.0
	gopkg.in/fsnotify.v1 v1.4.7
	gopkg.in/robfig/cron.v2 v2.0.0-20150107220207-be2e0b0deed5
	gopkg.in/yaml.v2 v2.3.0
	gopkg.in/yaml.v3 v3.0.0-20200615113413-eeeca48fe776
	k8s.io/api v0.19.3
	k8s.io/apimachinery v0.19.3
	k8s.io/client-go v0.19.3
	k8s.io/code-generator v0.19.3
	k8s.io/klog v1.0.0
	k8s.io/klog/v2 v2.2.0
	k8s.io/utils v0.0.0-20200912215256-4140de9c8800
	knative.dev/pkg v0.0.0-20210107022335-51c72e24c179
	mvdan.cc/xurls/v2 v2.0.0
	sigs.k8s.io/controller-runtime v0.7.0-alpha.6.0.20201106193838-8d0107636985
	sigs.k8s.io/yaml v1.2.0
)

go 1.13
