/*
Copyright 2019 The Kubernetes Authors.

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
	"flag"
	"fmt"
	"net/url"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/bugzilla"
	"k8s.io/test-infra/prow/config/secret"
)

// BugzillaOptions holds options for interacting with Bugzilla.
type BugzillaOptions struct {
	endpoint                string
	githubExternalTrackerId uint
	ApiKeyPath              string
	authMethod              string
}

// AddFlags injects Bugzilla options into the given FlagSet.
func (o *BugzillaOptions) AddFlags(fs *flag.FlagSet) {
	fs.StringVar(&o.endpoint, "bugzilla-endpoint", "", "Bugzilla's API endpoint.")
	fs.UintVar(&o.githubExternalTrackerId, "bugzilla-github-external-tracker-id", 0, "The ext_type_id for GitHub external bugs, optional.")
	fs.StringVar(&o.ApiKeyPath, "bugzilla-api-key-path", "", "Path to the file containing the Bugzilla API key.")
	fs.StringVar(&o.authMethod, "bugzilla-auth-method", "", "Which authorization method will be used. Values can be bearer, query or x-bugzilla-api-key.")
}

// Validate validates Bugzilla options.
func (o *BugzillaOptions) Validate(dryRun bool) error {
	if o.endpoint == "" {
		logrus.Info("empty -bugzilla-endpoint, will not create Bugzilla client")
		return nil
	}

	if _, err := url.ParseRequestURI(o.endpoint); err != nil {
		return fmt.Errorf("invalid -bugzilla-endpoint URI: %q", o.endpoint)
	}

	if o.ApiKeyPath == "" {
		logrus.Info("empty -bugzilla-api-key-path, will use anonymous Bugzilla client")
	}

	if o.authMethod != "" {
		if o.authMethod != "bearer" && o.authMethod != "query" && o.authMethod != "x-bugzilla-api-key" {
			return fmt.Errorf("invalid --auth-method %s. Valid values are bearer,query or x-bugzilla-api-key", o.authMethod)
		}
	}

	return nil
}

// BugzillaClient returns a Bugzilla client.
func (o *BugzillaOptions) BugzillaClient() (bugzilla.Client, error) {
	if o.endpoint == "" {
		return nil, fmt.Errorf("empty -bugzilla-endpoint, cannot create Bugzilla client")
	}

	var generator *func() []byte
	if o.ApiKeyPath == "" {
		generatorFunc := func() []byte {
			return []byte{}
		}
		generator = &generatorFunc
	} else {
		generatorFunc := secret.GetTokenGenerator(o.ApiKeyPath)
		generator = &generatorFunc
	}

	return bugzilla.NewClient(*generator, o.endpoint, o.githubExternalTrackerId, o.authMethod), nil
}
