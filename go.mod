// Please read https://git.k8s.io/test-infra/docs/dep.md before updating dependencies.

module k8s.io/test-infra

replace github.com/golang/lint => golang.org/x/lint v0.0.0-20190301231843-5614ed5bae6f

// Pin all k8s.io staging repositories to kubernetes v0.18.6
// When bumping Kubernetes dependencies, you should update each of these lines
// to point to the same kubernetes v0.KubernetesMinor.KubernetesPatch version
// before running update-deps.sh.
replace (
	cloud.google.com/go/pubsub => cloud.google.com/go/pubsub v1.3.1
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v14.2.0+incompatible
	github.com/googleapis/gnostic => github.com/googleapis/gnostic v0.4.1

	// Upstream is unmaintained. This fork introduces two important changes:
	// * We log an error if writing a cache key fails (e.G. because disk is full)
	// * We inject a header that allows ghproxy to detect if the response was revalidated or a cache miss
	github.com/gregjones/httpcache => github.com/alvaroaleman/httpcache v0.0.0-20210618195546-ab9a1a3f8a38

	golang.org/x/lint => golang.org/x/lint v0.0.0-20190409202823-959b441ac422
	gopkg.in/yaml.v3 => gopkg.in/yaml.v3 v3.0.0-20190709130402-674ba3eaed22
	k8s.io/client-go => k8s.io/client-go v0.22.2
)

require (
	cloud.google.com/go/container v1.2.0 // indirect
	cloud.google.com/go/kms v1.3.0 // indirect
	cloud.google.com/go/monitoring v1.3.0 // indirect
	cloud.google.com/go/pubsub v1.5.0
	cloud.google.com/go/secretmanager v1.2.0
	cloud.google.com/go/storage v1.12.0
	cloud.google.com/go/trace v1.1.0 // indirect
	github.com/Azure/azure-sdk-for-go v46.4.0+incompatible
	github.com/Azure/azure-storage-blob-go v0.8.0
	github.com/Azure/go-autorest/autorest v0.11.18
	github.com/Azure/go-autorest/autorest/adal v0.9.13
	github.com/GoogleCloudPlatform/testgrid v0.0.68
	github.com/NYTimes/gziphandler v1.1.1
	github.com/andygrunwald/go-gerrit v0.0.0-20210709065208-9d38b0be0268
	github.com/andygrunwald/go-jira v1.14.0
	github.com/aws/aws-sdk-go v1.37.22
	github.com/aws/aws-sdk-go-v2/config v1.14.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/ecr v1.15.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/ecrpublic v1.12.0 // indirect
	github.com/awslabs/amazon-ecr-credential-helper/ecr-login v0.0.0-20220216180153-3d7835abdf40
	github.com/bazelbuild/buildtools v0.0.0-20200922170545-10384511ce98
	github.com/blang/semver v3.5.1+incompatible
	github.com/bwmarrin/snowflake v0.0.0
	github.com/chrismellard/docker-credential-acr-env v0.0.0-20220119192733-fe33c00cee21
	github.com/clarketm/json v1.13.4
	github.com/client9/misspell v0.3.4
	github.com/danwakefield/fnmatch v0.0.0-20160403171240-cbb64ac3d964 // indirect
	github.com/denormal/go-gitignore v0.0.0-20180930084346-ae8ad1d07817
	github.com/dgrijalva/jwt-go/v4 v4.0.0-preview1
	github.com/djherbis/atime v1.0.0
	github.com/evanphx/json-patch v4.11.0+incompatible
	github.com/felixge/fgprof v0.9.1
	github.com/fsnotify/fsnotify v1.5.1
	github.com/fsouza/fake-gcs-server v1.19.4
	github.com/go-bindata/go-bindata/v3 v3.1.3
	github.com/go-openapi/spec v0.19.6
	github.com/go-test/deep v1.0.7
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/gomodule/redigo v1.8.5
	github.com/google/go-cmp v0.5.7
	github.com/google/go-containerregistry v0.8.1-0.20220209165246-a44adc326839
	github.com/google/go-github v17.0.0+incompatible
	github.com/google/gofuzz v1.2.1-0.20210504230335-f78f29fc09ea
	github.com/google/uuid v1.3.0
	github.com/gorilla/csrf v1.6.2
	github.com/gorilla/mux v1.8.0
	github.com/gorilla/securecookie v1.1.1
	github.com/gorilla/sessions v1.2.0
	github.com/gregjones/httpcache v0.0.0-20190611155906-901d90724c79
	github.com/hashicorp/go-retryablehttp v0.6.6
	github.com/hashicorp/golang-lru v0.5.4
	github.com/klauspost/pgzip v1.2.1
	github.com/mattn/go-zglob v0.0.2
	github.com/pelletier/go-toml v1.9.4
	github.com/peterbourgon/diskv v2.0.1+incompatible
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.11.0
	github.com/prometheus/client_model v0.2.0
	github.com/prometheus/common v0.26.0
	github.com/satori/go.uuid v1.2.0
	github.com/shurcooL/githubv4 v0.0.0-20210725200734-83ba7b4c9228
	github.com/sirupsen/logrus v1.8.1
	github.com/spf13/cobra v1.3.0
	github.com/spf13/pflag v1.0.5
	github.com/tektoncd/pipeline v0.14.1-0.20200710073957-5eeb17f81999
	go.uber.org/zap v1.19.0
	go4.org v0.0.0-20201209231011-d4a079459e60
	gocloud.dev v0.19.0
	golang.org/x/crypto v0.0.0-20210921155107-089bfa567519
	golang.org/x/lint v0.0.0-20210508222113-6edffad5e616
	golang.org/x/net v0.0.0-20220127200216-cd36cc0744dd
	golang.org/x/oauth2 v0.0.0-20211104180415-d3ed0bb246c8
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	golang.org/x/sys v0.0.0-20220224120231-95c6836cb0e7 // indirect
	golang.org/x/time v0.0.0-20210723032227-1f47c861a9ac
	golang.org/x/tools v0.1.8
	google.golang.org/api v0.70.0
	google.golang.org/genproto v0.0.0-20220222213610-43724f9ea8cf
	gopkg.in/fsnotify.v1 v1.4.7
	gopkg.in/ini.v1 v1.66.2
	gopkg.in/robfig/cron.v2 v2.0.0-20150107220207-be2e0b0deed5
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b
	k8s.io/api v0.22.2
	k8s.io/apimachinery v0.22.2
	k8s.io/client-go v11.0.1-0.20190805182717-6502b5e7b1b5+incompatible
	k8s.io/code-generator v0.22.2
	k8s.io/klog v1.0.0
	k8s.io/utils v0.0.0-20210819203725-bdf08cb9a70a
	knative.dev/pkg v0.0.0-20200711004937-22502028e31a
	mvdan.cc/xurls/v2 v2.0.0
	sigs.k8s.io/boskos v0.0.0-20210730172138-093b54882439
	sigs.k8s.io/controller-runtime v0.10.3
	sigs.k8s.io/controller-tools v0.6.3-0.20210827222652-7b3a8699fa04
	sigs.k8s.io/yaml v1.2.0
)

go 1.16
