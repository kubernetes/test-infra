// Please read https://git.k8s.io/test-infra/docs/dep.md before updating dependencies.

module k8s.io/test-infra/triage

go 1.18

require (
	k8s.io/apimachinery v0.22.0
	k8s.io/klog/v2 v2.10.0
)

require github.com/go-logr/logr v0.4.0 // indirect
