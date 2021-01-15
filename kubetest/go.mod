module k8s.io/test-infra/kubetest

go 1.16

replace k8s.io/test-infra => ../

replace k8s.io/client-go => k8s.io/client-go v0.19.3

replace k8s.io/api => k8s.io/api v0.19.3

require (
	cloud.google.com/go/storage v1.12.0
	github.com/Azure/azure-sdk-for-go v49.2.0+incompatible
	github.com/Azure/azure-storage-blob-go v0.12.0
	github.com/Azure/go-autorest/autorest v0.11.15
	github.com/Azure/go-autorest/autorest/adal v0.9.10
	github.com/aws/aws-sdk-go v1.36.23
	github.com/docker/docker v20.10.2+incompatible
	github.com/pelletier/go-toml v1.8.1
	github.com/satori/go.uuid v1.2.0
	github.com/spf13/pflag v1.0.5
	golang.org/x/crypto v0.0.0-20201221181555-eec23a3978ad
	k8s.io/api v0.19.3
	k8s.io/apimachinery v0.20.1
	k8s.io/client-go v11.0.1-0.20190805182717-6502b5e7b1b5+incompatible
	sigs.k8s.io/boskos v0.0.0-20210106090752-ee7838a6f6ef
)
