/*
Copyright 2017 The Kubernetes Authors.

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

package main

import (
	"flag"
	"os"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/maintenance/migratestatus/migrator"
	"k8s.io/test-infra/prow/config"
	prowflagutil "k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/logrusutil"
)

type options struct {
	org, repo                                            string
	copyContext, moveContext, retireContext, destContext string
	descriptionURL                                       string
	continueOnError, dryRun                              bool
	github                                               prowflagutil.GitHubOptions
}

func gatherOptions() options {
	o := options{}
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	fs.StringVar(&o.org, "org", "", "The organization that owns the repo.")
	fs.StringVar(&o.repo, "repo", "", "The repo needing status migration.")
	fs.BoolVar(&o.dryRun, "dry-run", true, "Run in dry-run mode, performing no modifying actions.")
	fs.BoolVar(&o.continueOnError, "continue-on-error", false, "Indicates that the migration should continue if context migration fails for an individual PR.")

	fs.StringVar(&o.copyContext, "copy", "", "Indicates copy mode and specifies the context to copy.")
	fs.StringVar(&o.moveContext, "move", "", "Indicates move mode and specifies the context to move.")
	fs.StringVar(&o.retireContext, "retire", "", "Indicates retire mode and specifies the context to retire.")
	fs.StringVar(&o.destContext, "dest", "", "The destination context to copy or move to. For retire mode this is the context that replaced the retired context.")
	fs.StringVar(&o.descriptionURL, "description", "", "A URL to a page explaining why a context was migrated or retired. (Optional)")

	o.github.AddFlags(fs)
	fs.Parse(os.Args[1:])
	return o
}

func (o *options) Validate() error {
	if o.org == "" {
		return errors.New("'--org' must be set.\n")
	}
	if o.repo == "" {
		return errors.New("'--repo' must be set.\n")
	}

	if o.destContext == "" && o.retireContext == "" {
		return errors.New("'--dest' is required unless using '--retire' mode.\n")
	}

	if o.descriptionURL != "" && o.copyContext != "" {
		return errors.New("'--description' URL is not applicable to '--copy' mode")
	}

	var optionCount int
	if o.copyContext != "" {
		optionCount++
	}
	if o.moveContext != "" {
		optionCount++
	}
	if o.retireContext != "" {
		optionCount++
	}
	if optionCount != 1 {
		return errors.New("Exactly one mode must be specified [--copy|--retire|--move].")
	}

	if err := o.github.Validate(o.dryRun); err != nil {
		return err
	}
	return nil
}

func main() {
	o := gatherOptions()
	if err := o.Validate(); err != nil {
		logrus.WithError(err).Fatal("Invalid options")
	}

	logrus.SetFormatter(
		logrusutil.NewDefaultFieldsFormatter(nil, logrus.Fields{"component": "migratestatus"}),
	)

	secretAgent := &config.SecretAgent{}
	if o.github.TokenPath != "" {
		if err := secretAgent.Start([]string{o.github.TokenPath}); err != nil {
			logrus.WithError(err).Fatal("Error starting secrets agent.")
		}
	}

	githubClient, err := o.github.GitHubClient(secretAgent, o.dryRun)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting GitHub client.")
	}

	var mode *migrator.Mode
	if o.copyContext != "" {
		mode = migrator.CopyMode(o.copyContext, o.destContext)
	}
	if o.moveContext != "" {
		mode = migrator.MoveMode(o.moveContext, o.destContext, o.descriptionURL)
	}
	if o.retireContext != "" {
		mode = migrator.RetireMode(o.retireContext, o.destContext, o.descriptionURL)
	}

	// Note that continueOnError is false by default so that errors can be addressed when they occur
	// instead of blindly continuing to the next PR, possibly continuing to error.
	m := migrator.New(*mode, githubClient, o.org, o.repo, o.continueOnError)
	if err := m.Migrate(); err != nil {
		logrus.WithError(err).Fatal("Error during status migration")
	}
	os.Exit(0)
}
