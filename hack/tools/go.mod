module k8s.io/test-infra/hack/tools

go 1.16

require (
	cloud.google.com/go v0.102.0 // indirect
	github.com/bazelbuild/buildtools v0.0.0-20211007154642-8dd79e56e98e
	github.com/client9/misspell v0.3.4
	github.com/go-bindata/go-bindata/v3 v3.1.3
	github.com/golang/protobuf v1.5.2
	// There is no release of golangci-lint with staticcheck for go 1.18 enabled but support for it is
	// already merged. Use an unreleased version as that is probably the single most important linter.
	github.com/golangci/golangci-lint v1.45.3-0.20220409135141-1643bd09f2b4
	github.com/google/ko v0.11.2
	github.com/sethvargo/gcs-cacher v0.1.3
	golang.org/x/oauth2 v0.0.0-20220608161450-d0670ef3b1eb // indirect
	golang.org/x/sys v0.0.0-20220608164250-635b8c9b7f68 // indirect
	golang.org/x/time v0.0.0-20220411224347-583f2d630306 // indirect
	google.golang.org/api v0.83.0 // indirect
	google.golang.org/genproto v0.0.0-20220608133413-ed9918b62aac // indirect
	gotest.tools/gotestsum v1.7.0
	k8s.io/api v0.22.2 // indirect
	k8s.io/code-generator v0.21.4
	sigs.k8s.io/controller-tools v0.6.3-0.20210827222652-7b3a8699fa04
)
