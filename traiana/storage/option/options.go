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

func GetAws(opts []ClientOption) awsapi.ClientOption {
	if len(opts) == 1 {
		return opts[0].Aws
	}

	if len(opts) == 0 {
		return awsapi.ClientOption{}
	}

	panic("multiple awsapi.ClientOption not supported")
}

func GetGcs(opts []ClientOption) []option.ClientOption {
	g := make([]option.ClientOption, len(opts))

	for i := range opts {
		g[i] = opts[i].Gcs
	}

	return g
}

func WithoutAuthentication() ClientOption {
	var o ClientOption

	if (traiana.Aws) {
		o.Aws = awsapi.ClientOption{
			NoAuth: true,
		}

	} else {
		o.Gcs = option.WithoutAuthentication()
	}

	return o
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
