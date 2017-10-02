package main

import (
	"k8s.io/test-infra/prow/config"
)

type Config struct {
	config.Config `json:",inline"`

	JenkinsProxy proxy `json:"jenkins_proxy"`
}
