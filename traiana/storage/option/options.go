package option

import (
	"google.golang.org/api/option"
	"k8s.io/test-infra/traiana"
	"k8s.io/test-infra/traiana/awsapi"
)

type ClientOption struct {
	Aws awsapi.ClientOption
	Gcs option.ClientOption
}

func WithCredentialsFile(credentialsFile string) ClientOption {
	var o ClientOption

	if (traiana.Aws) {
		o.Aws = awsapi.ClientOption{
			CredentialsFile: credentialsFile,
		}

	} else {
		o.Gcs = option.WithCredentialsFile(credentialsFile)
	}

	return o
}
