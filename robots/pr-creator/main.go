/*
Copyright 2018 The Kubernetes Authors.

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
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config/secret"
	"k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/robots/pr-creator/updater"
)

type options struct {
	github flagutil.GitHubOptions

	branch    string
	allowMods bool
	confirm   bool
	local     bool
	org       string
	repo      string
	source    string

	title      string
	headBranch string
	body       string
}

func (o options) validate() error {
	switch {
	case o.org == "":
		return errors.New("--org must be set")
	case o.repo == "":
		return errors.New("--repo must be set")
	case o.branch == "":
		return errors.New("--branch must be set")
	case o.source == "":
		return errors.New("--source must be set")
	case !o.local && !strings.Contains(o.source, ":"):
		return fmt.Errorf("--source=%s requires --local", o.source)
	}
	if err := o.github.Validate(!o.confirm); err != nil {
		return err
	}
	return nil
}

func optionsFromFlags() options {
	var o options
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	o.github.AddFlags(fs)
	fs.StringVar(&o.repo, "repo", "", "GitHub repo")
	fs.StringVar(&o.org, "org", "", "GitHub org")
	fs.StringVar(&o.branch, "branch", "", "Repo branch to merge into")
	fs.StringVar(&o.source, "source", "", "The user:branch to merge from")

	fs.BoolVar(&o.allowMods, "allow-mods", updater.PreventMods, "Indicates whether maintainers can modify the pull request")
	fs.BoolVar(&o.confirm, "confirm", false, "Set to mutate github instead of a dry run")
	fs.BoolVar(&o.local, "local", false, "Allow source to be local-branch instead of remote-user:branch")
	fs.StringVar(&o.title, "title", "", "Title of PR")
	fs.StringVar(&o.headBranch, "head-branch", "", "Reuse any self-authored open PR from this branch")
	fs.StringVar(&o.body, "body", "", "Body of PR")
	fs.Parse(os.Args[1:])
	return o
}

func main() {
	o := optionsFromFlags()
	if err := o.validate(); err != nil {
		logrus.WithError(err).Fatal("bad flags")
	}

	jamesBond := &secret.Agent{}
	if err := jamesBond.Start([]string{o.github.TokenPath}); err != nil {
		logrus.WithError(err).Fatal("Failed to start secrets agent")
	}

	gc, err := o.github.GitHubClient(jamesBond, !o.confirm)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to create github client")
	}

	n, err := updater.EnsurePR(o.org, o.repo, o.title, o.body, o.source, o.branch, o.headBranch, o.allowMods, gc)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to ensure PR exists.")
	}

	logrus.Infof("PR %s/%s#%d will merge %s into %s: %s", o.org, o.repo, *n, o.source, o.branch, o.title)
	fmt.Println(*n)
}
