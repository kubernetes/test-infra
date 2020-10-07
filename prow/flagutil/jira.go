/*
Copyright 2020 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package flagutil

import (
	"errors"
	"flag"
	"fmt"
	"net/url"

	"k8s.io/test-infra/prow/config/secret"
	"k8s.io/test-infra/prow/jira"
)

type JiraOptions struct {
	endpoint     string
	username     string
	passwordFile string
}

func (o *JiraOptions) AddFlags(fs *flag.FlagSet) {
	fs.StringVar(&o.endpoint, "jira-endpoint", "", "The Jira endpoint to use")
	fs.StringVar(&o.username, "jira-username", "", "The username to use for Jira basic auth")
	fs.StringVar(&o.passwordFile, "jira-password-file", "", "Location to a file containing the Jira basic auth password")
}

func (o *JiraOptions) Validate(_ bool) error {
	if o.endpoint == "" {
		return nil
	}

	if _, err := url.ParseRequestURI(o.endpoint); err != nil {
		return fmt.Errorf("--jira-endpoint %q is invalid: %w", o.endpoint, err)
	}

	if (o.username != "") != (o.passwordFile != "") {
		return errors.New("--jira-username and --jira-password-file must be specified together")
	}

	return nil
}

func (o *JiraOptions) Client(secretAgent *secret.Agent) (jira.Client, error) {
	if o.endpoint == "" {
		return nil, errors.New("empty --jira-endpoint, can not create a client")
	}

	var opts []jira.Option
	if o.passwordFile != "" {
		if err := secretAgent.Add(o.passwordFile); err != nil {
			return nil, fmt.Errorf("failed to get --jira-password-file: %w", err)
		}
		opts = append(opts, jira.WithBasicAuth(func() (string, string) {
			return o.username, string(secretAgent.GetSecret(o.passwordFile))
		}))
	}

	return jira.NewClient(o.endpoint, opts...)
}
