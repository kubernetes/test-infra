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

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/github"

	"github.com/sirupsen/logrus"
)

type options struct {
	github flagutil.GitHubOptions

	branch  string
	confirm bool
	local   bool
	org     string
	repo    string
	source  string

	title      string
	matchTitle string
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
	fs.StringVar(&o.repo, "repo", "", "Github repo")
	fs.StringVar(&o.org, "org", "", "Github org")
	fs.StringVar(&o.branch, "branch", "", "Repo branch to merge into")
	fs.StringVar(&o.source, "source", "", "The user:branch to merge from")

	fs.BoolVar(&o.confirm, "confirm", false, "Set to mutate github instead of a dry run")
	fs.BoolVar(&o.local, "local", false, "Allow source to be local-branch instead of remote-user:branch")
	fs.StringVar(&o.title, "title", "", "Title of PR")
	fs.StringVar(&o.matchTitle, "match-title", "", "Reuse any self-authored, open PR matching title")
	fs.StringVar(&o.body, "body", "", "Body of PR")
	fs.Parse(os.Args[1:])
	return o
}

func main() {
	o := optionsFromFlags()
	if err := o.validate(); err != nil {
		logrus.WithError(err).Fatal("bad flags")
	}

	var jamesBond config.SecretAgent
	if err := jamesBond.Start([]string{o.github.TokenPath}); err != nil {
		logrus.WithError(err).Fatal("Failed to start secrets agent")
	}

	gc, err := o.github.GitHubClient(&jamesBond, !o.confirm)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to create github client")
	}

	n, err := updatePR(o, gc)
	if err != nil {
		logrus.WithError(err).Fatalf("Failed to update %d", n)
	}
	if n == nil {
		allowMods := true
		pr, err := gc.CreatePullRequest(o.org, o.repo, o.title, o.body, o.source, o.branch, allowMods)
		if err != nil {
			logrus.WithError(err).Fatal("Failed to create PR")
		}
		n = &pr
	}

	logrus.Infof("PR %s/%s#%d will merge %s into %s: %s", o.org, o.repo, *n, o.source, o.branch, o.title)

	fmt.Println(*n)
}

type updateClient interface {
	UpdatePullRequest(org, repo string, number int, title, body *string, open *bool, branch *string, canModify *bool) error
	BotName() (string, error)
	FindIssues(query, sort string, asc bool) ([]github.Issue, error)
}

func updatePR(o options, gc updateClient) (*int, error) {
	if o.matchTitle == "" {
		return nil, nil
	}

	logrus.Info("Looking for a PR to reuse...")
	me, err := gc.BotName()
	if err != nil {
		return nil, fmt.Errorf("bot name: %v", err)
	}

	issues, err := gc.FindIssues("is:open is:pr archived:false in:title author:"+me+" "+o.matchTitle, "updated", true)
	if err != nil {
		return nil, fmt.Errorf("find issues: %v", err)
	} else if len(issues) == 0 {
		logrus.Info("No reusable issues found")
		return nil, nil
	}
	n := issues[0].Number
	logrus.Infof("Found %d", n)
	var ignoreOpen *bool
	var ignoreBranch *string
	var ignoreModify *bool
	if err := gc.UpdatePullRequest(o.org, o.repo, n, &o.title, &o.body, ignoreOpen, ignoreBranch, ignoreModify); err != nil {
		return nil, fmt.Errorf("update %d: %v", n, err)
	}

	return &n, nil
}
