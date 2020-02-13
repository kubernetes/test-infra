module k8s.io/test-infra

replace github.com/golang/lint => golang.org/x/lint v0.0.0-20190301231843-5614ed5bae6f

// Pin all k8s.io staging repositories to kubernetes-1.15.3.
// When bumping Kubernetes dependencies, you should update each of these lines
// to point to the same kubernetes-1.x.y release branch before running update-deps.sh.
replace (
	cloud.google.com/go => cloud.google.com/go v0.44.3
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v12.2.0+incompatible
	golang.org/x/lint => golang.org/x/lint v0.0.0-20190409202823-959b441ac422
	k8s.io/api => k8s.io/api v0.0.0-20190918195907-bd6ac527cfd2
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.0.0-20190918201827-3de75813f604
	k8s.io/apimachinery => k8s.io/apimachinery v0.0.0-20190817020851-f2f3a405f61d
	k8s.io/client-go => k8s.io/client-go v0.0.0-20190918200256-06eb1244587a
	k8s.io/code-generator => k8s.io/code-generator v0.0.0-20190612205613-18da4a14b22b
)

require (
	cloud.google.com/go v0.47.0
	github.com/Azure/azure-pipeline-go v0.1.9 // indirect
	github.com/Azure/azure-sdk-for-go v38.0.0+incompatible
	github.com/Azure/azure-storage-blob-go v0.0.0-20190123011202-457680cc0804
	github.com/Azure/go-autorest/autorest v0.9.5
	github.com/Azure/go-autorest/autorest/adal v0.8.2
	github.com/GoogleCloudPlatform/testgrid v0.0.1-alpha.4
	github.com/NYTimes/gziphandler v0.0.0-20170623195520-56545f4a5d46
	github.com/andygrunwald/go-gerrit v0.0.0-20190120104749-174420ebee6c
	github.com/aws/aws-k8s-tester v0.0.0-20190114231546-b411acf57dfe
	github.com/aws/aws-sdk-go v1.27.4
	github.com/bazelbuild/buildtools v0.0.0-20190917191645-69366ca98f89
	github.com/blang/semver v3.5.1+incompatible
	github.com/bwmarrin/snowflake v0.0.0
	github.com/clarketm/json v1.13.4
	github.com/client9/misspell v0.3.4
	github.com/containerd/containerd v1.3.3 // indirect
	github.com/denisenkom/go-mssqldb v0.0.0-20190111225525-2fea367d496d // indirect
	github.com/djherbis/atime v1.0.0
	github.com/docker/docker v1.4.2-0.20190924003213-a8608b5b67c7
	github.com/erikstmartin/go-testdb v0.0.0-20160219214506-8d10e4a1bae5 // indirect
	github.com/evanphx/json-patch v4.5.0+incompatible
	github.com/fsnotify/fsnotify v1.4.7
	github.com/fsouza/fake-gcs-server v0.0.0-20180612165233-e85be23bdaa8
	github.com/go-logr/zapr v0.1.1 // indirect
	github.com/go-openapi/spec v0.19.6
	github.com/go-openapi/swag v0.19.7 // indirect
	github.com/go-sql-driver/mysql v0.0.0-20160411075031-7ebe0a500653 // indirect
	github.com/go-test/deep v1.0.4
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/golang/mock v1.3.1
	github.com/golang/protobuf v1.3.3 // indirect
	github.com/gomodule/redigo v1.7.0
	github.com/google/go-cmp v0.3.1
	github.com/google/go-github v17.0.0+incompatible
	github.com/google/go-querystring v1.0.0 // indirect
	github.com/google/uuid v1.1.1
	github.com/gorilla/csrf v1.6.2
	github.com/gorilla/securecookie v1.1.1
	github.com/gorilla/sessions v1.1.3
	github.com/gregjones/httpcache v0.0.0-20190212212710-3befbb6ad0cc
	github.com/hashicorp/go-multierror v1.0.0
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/influxdata/influxdb v0.0.0-20161215172503-049f9b42e9a5
	github.com/jinzhu/gorm v0.0.0-20170316141641-572d0a0ab1eb
	github.com/jinzhu/inflection v0.0.0-20190603042836-f5c5f50e6090 // indirect
	github.com/jinzhu/now v1.0.1 // indirect
	github.com/json-iterator/go v1.1.9 // indirect
	github.com/klauspost/compress v1.4.1 // indirect
	github.com/klauspost/cpuid v1.2.3 // indirect
	github.com/klauspost/pgzip v1.2.1
	github.com/lib/pq v1.0.0 // indirect
	github.com/magiconair/properties v1.8.1 // indirect
	github.com/mattn/go-sqlite3 v0.0.0-20160514122348-38ee283dabf1 // indirect
	github.com/mattn/go-zglob v0.0.1
	github.com/pelletier/go-toml v1.3.0
	github.com/peterbourgon/diskv v2.0.1+incompatible
	github.com/pkg/errors v0.8.1
	github.com/prometheus/client_golang v1.1.0
	github.com/prometheus/client_model v0.0.0-20190129233127-fd36f4220a90
	github.com/prometheus/common v0.7.0
	github.com/prometheus/procfs v0.0.8 // indirect
	github.com/satori/go.uuid v1.2.0
	github.com/shurcooL/githubv4 v0.0.0-20191102174205-af46314aec7b
	github.com/sirupsen/logrus v1.4.2
	github.com/spf13/cast v1.3.1 // indirect
	github.com/spf13/cobra v0.0.5
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.3.2
	github.com/tektoncd/pipeline v0.10.1
	go.opencensus.io v0.22.3 // indirect
	golang.org/x/crypto v0.0.0-20191206172530-e9b2fee46413
	golang.org/x/lint v0.0.0-20191125180803-fdd1cda4f05f
	golang.org/x/net v0.0.0-20191119073136-fc4aabc6c914
	golang.org/x/oauth2 v0.0.0-20190604053449-0f29369cfe45
	golang.org/x/sync v0.0.0-20190911185100-cd5d95a43a6e
	golang.org/x/time v0.0.0-20191024005414-555d28b269f0
	golang.org/x/tools v0.0.0-20200115165105-de0b1760071a
	google.golang.org/api v0.10.0
	gopkg.in/robfig/cron.v2 v2.0.0-20150107220207-be2e0b0deed5
	gopkg.in/yaml.v2 v2.2.8 // indirect
	gopkg.in/yaml.v3 v3.0.0-20190709130402-674ba3eaed22
	k8s.io/api v0.17.2
	k8s.io/apimachinery v0.17.2
	k8s.io/client-go v11.0.1-0.20190805182717-6502b5e7b1b5+incompatible
	k8s.io/code-generator v0.17.1
	k8s.io/klog v1.0.0
	k8s.io/utils v0.0.0-20191114184206-e782cd3c129f
	knative.dev/pkg v0.0.0-20191111150521-6d806b998379
	mvdan.cc/xurls/v2 v2.0.0
	sigs.k8s.io/controller-runtime v0.3.0
	sigs.k8s.io/yaml v1.1.0
)

go 1.13
