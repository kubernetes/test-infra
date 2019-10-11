module k8s.io/test-infra

replace github.com/golang/lint => golang.org/x/lint v0.0.0-20190301231843-5614ed5bae6f

// Pin all k8s.io staging repositories to kubernetes-1.15.3.
// When bumping Kubernetes dependencies, you should update each of these lines
// to point to the same kubernetes-1.x.y release branch before running update-deps.sh.
replace (
	k8s.io/api => k8s.io/api v0.0.0-20190918195907-bd6ac527cfd2
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.0.0-20190918201827-3de75813f604
	k8s.io/apimachinery => k8s.io/apimachinery v0.0.0-20190817020851-f2f3a405f61d
	k8s.io/client-go => k8s.io/client-go v0.0.0-20190918200256-06eb1244587a
	k8s.io/code-generator => k8s.io/code-generator v0.0.0-20190612205613-18da4a14b22b
)

require (
	cloud.google.com/go v0.44.3
	github.com/Azure/azure-sdk-for-go v21.1.0+incompatible
	github.com/Azure/azure-storage-blob-go v0.0.0-20190123011202-457680cc0804
	github.com/Azure/go-autorest v11.1.2+incompatible
	github.com/GoogleCloudPlatform/testgrid v0.0.0-20191002194340-462e7a9505a0
	github.com/NYTimes/gziphandler v0.0.0-20170623195520-56545f4a5d46
	github.com/andygrunwald/go-gerrit v0.0.0-20190120104749-174420ebee6c
	github.com/aws/aws-k8s-tester v0.0.0-20190114231546-b411acf57dfe
	github.com/aws/aws-sdk-go v1.16.36
	github.com/bazelbuild/bazel-gazelle v0.18.1
	github.com/bazelbuild/buildtools v0.0.0-20190404153937-93253d6efaa9
	github.com/bwmarrin/snowflake v0.0.0
	github.com/clarketm/json v1.13.0
	github.com/client9/misspell v0.3.4
	github.com/djherbis/atime v1.0.0
	github.com/docker/docker v0.7.3-0.20190327010347-be7ac8be2ae0
	github.com/evanphx/json-patch v4.5.0+incompatible
	github.com/fsnotify/fsnotify v1.4.7
	github.com/fsouza/fake-gcs-server v0.0.0-20180612165233-e85be23bdaa8
	github.com/go-openapi/spec v0.19.2
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/golang/mock v1.3.1
	github.com/golang/protobuf v1.3.2
	github.com/gomodule/redigo v1.7.0
	github.com/google/go-cmp v0.3.1
	github.com/google/go-github v17.0.0+incompatible
	github.com/google/uuid v1.1.1
	github.com/gorilla/csrf v1.6.1
	github.com/gorilla/securecookie v1.1.1
	github.com/gorilla/sessions v1.1.3
	github.com/gregjones/httpcache v0.0.0-20190212212710-3befbb6ad0cc
	github.com/hashicorp/go-multierror v0.0.0-20171204182908-b7773ae21874
	github.com/influxdata/influxdb v0.0.0-20161215172503-049f9b42e9a5
	github.com/jinzhu/gorm v0.0.0-20170316141641-572d0a0ab1eb
	github.com/klauspost/pgzip v1.2.1
	github.com/knative/build v0.3.1-0.20190330033454-38ace00371c7
	github.com/knative/pkg v0.0.0-20190330034653-916205998db9
	github.com/mattn/go-zglob v0.0.1
	github.com/morikuni/aec v0.0.0-20170113033406-39771216ff4c // indirect
	github.com/pelletier/go-toml v1.3.0
	github.com/peterbourgon/diskv v2.0.1+incompatible
	github.com/pkg/errors v0.8.1
	github.com/prometheus/client_golang v0.9.4
	github.com/prometheus/client_model v0.0.0-20190129233127-fd36f4220a90
	github.com/satori/go.uuid v0.0.0-20160713180306-0aa62d5ddceb
	github.com/shurcooL/githubv4 v0.0.0-20180925043049-51d7b505e2e9
	github.com/sirupsen/logrus v1.4.2
	github.com/spf13/cobra v0.0.5
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.3.2
	github.com/tektoncd/pipeline v0.1.1-0.20190327171839-7c43fbae2816
	golang.org/x/crypto v0.0.0-20190611184440-5c40567a22f8
	golang.org/x/lint v0.0.0-20190409202823-959b441ac422
	golang.org/x/net v0.0.0-20190827160401-ba9fcec4b297
	golang.org/x/oauth2 v0.0.0-20190604053449-0f29369cfe45
	golang.org/x/sync v0.0.0-20190423024810-112230192c58
	golang.org/x/time v0.0.0-20190308202827-9d24e82272b4
	golang.org/x/tools v0.0.0-20190628153133-6cdbf07be9d0
	google.golang.org/api v0.9.0
	gopkg.in/robfig/cron.v2 v2.0.0-20150107220207-be2e0b0deed5
	gopkg.in/yaml.v3 v3.0.0-20190709130402-674ba3eaed22
	k8s.io/api v0.0.0-20190918195907-bd6ac527cfd2
	k8s.io/apiextensions-apiserver v0.0.0-20190918201827-3de75813f604
	k8s.io/apimachinery v0.0.0-20190817020851-f2f3a405f61d
	k8s.io/client-go v11.0.1-0.20190805182717-6502b5e7b1b5+incompatible
	k8s.io/code-generator v0.0.0-20190831074504-732c9ca86353
	k8s.io/klog v0.4.0
	k8s.io/repo-infra v0.0.0-20190921032325-1fedfadec8ce
	k8s.io/utils v0.0.0-20190506122338-8fab8cb257d5
	mvdan.cc/xurls/v2 v2.0.0
	sigs.k8s.io/controller-runtime v0.2.1
	sigs.k8s.io/yaml v1.1.0
)

go 1.13
