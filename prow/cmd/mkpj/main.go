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
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	"sigs.k8s.io/yaml"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/config/secret"
	prowflagutil "k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pjutil"
)

type options struct {
	jobName       string
	configPath    string
	jobConfigPath string

	baseRef    string
	baseSha    string
	pullNumber int
	pullSha    string
	pullAuthor string
	org        string
	repo       string

	github       prowflagutil.GitHubOptions
	githubClient githubClient
	pullRequest  *github.PullRequest
}

func (o *options) getPullRequest() (*github.PullRequest, error) {
	if o.pullRequest != nil {
		return o.pullRequest, nil
	}
	pr, err := o.githubClient.GetPullRequest(o.org, o.repo, o.pullNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch PullRequest from Github: %v", err)
	}
	o.pullRequest = pr
	return pr, nil
}

func (o *options) defaultPR(pjs *prowapi.ProwJobSpec) error {
	if pjs.Refs.Pulls[0].Number == 0 {
		fmt.Fprint(os.Stderr, "PR Number: ")
		var pullNumber int
		fmt.Scanln(&pullNumber)
		pjs.Refs.Pulls[0].Number = pullNumber
		o.pullNumber = pullNumber
	}
	if pjs.Refs.Pulls[0].Author == "" {
		pr, err := o.getPullRequest()
		if err != nil {
			return err
		}
		pjs.Refs.Pulls[0].Author = pr.User.Login
	}
	if pjs.Refs.Pulls[0].SHA == "" {
		pr, err := o.getPullRequest()
		if err != nil {
			return err
		}
		pjs.Refs.Pulls[0].SHA = pr.Head.SHA
	}
	return nil
}

func (o *options) defaultBaseRef(pjs *prowapi.ProwJobSpec) error {
	if pjs.Refs.BaseRef == "" {
		if o.pullNumber != 0 {
			pr, err := o.getPullRequest()
			if err != nil {
				return err
			}
			pjs.Refs.BaseRef = pr.Base.Ref
		} else {
			fmt.Fprint(os.Stderr, "Base ref (e.g. master): ")
			fmt.Scanln(&pjs.Refs.BaseRef)
		}
	}
	if pjs.Refs.BaseSHA == "" {
		if o.pullNumber != 0 {
			pr, err := o.getPullRequest()
			if err != nil {
				return err
			}
			pjs.Refs.BaseSHA = pr.Base.SHA
		} else {
			baseSHA, err := o.githubClient.GetRef(o.org, o.repo, fmt.Sprintf("heads/%s", pjs.Refs.BaseRef))
			if err != nil {
				logrus.Fatalf("failed to get base sha: %v", err)
				return err
			}
			pjs.Refs.BaseSHA = baseSHA
		}
	}
	return nil
}

type githubClient interface {
	GetPullRequest(org, repo string, number int) (*github.PullRequest, error)
	GetRef(org, repo, ref string) (string, error)
}

func (o *options) Validate() error {
	if o.jobName == "" {
		return errors.New("required flag --job was unset")
	}

	if o.configPath == "" {
		return errors.New("required flag --config-path was unset")
	}

	return nil
}

func gatherOptions() options {
	o := options{}
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	fs.StringVar(&o.jobName, "job", "", "Job to run.")
	fs.StringVar(&o.configPath, "config-path", "", "Path to config.yaml.")
	fs.StringVar(&o.jobConfigPath, "job-config-path", "", "Path to prow job configs.")
	fs.StringVar(&o.baseRef, "base-ref", "", "Git base ref under test")
	fs.StringVar(&o.baseSha, "base-sha", "", "Git base SHA under test")
	fs.IntVar(&o.pullNumber, "pull-number", 0, "Git pull number under test")
	fs.StringVar(&o.pullSha, "pull-sha", "", "Git pull SHA under test")
	fs.StringVar(&o.pullAuthor, "pull-author", "", "Git pull author under test")
	o.github.AddFlagsWithoutDefaultGithubTokenPath(fs)
	fs.Parse(os.Args[1:])
	return o
}

func main() {
	o := gatherOptions()
	if err := o.Validate(); err != nil {
		logrus.Fatalf("Invalid options: %v", err)
	}

	conf, err := config.Load(o.configPath, o.jobConfigPath)
	if err != nil {
		logrus.WithError(err).Fatal("Error loading config.")
	}

	var secretAgent *secret.Agent
	if o.github.TokenPath != "" {
		secretAgent = &secret.Agent{}
		if err := secretAgent.Start([]string{o.github.TokenPath}); err != nil {
			logrus.Fatalf("Failed to start secret agent: %v", err)
		}
	}
	o.githubClient, err = o.github.GitHubClient(secretAgent, false)
	if err != nil {
		logrus.Fatalf("failed to get Github client: %v", err)
	}

	var pjs prowapi.ProwJobSpec
	var labels map[string]string
	var found bool
	var needsBaseRef bool
	var needsPR bool
	for fullRepoName, ps := range conf.Presubmits {
		org, repo, err := splitRepoName(fullRepoName)
		if err != nil {
			logrus.WithError(err).Warnf("Invalid repo name %s.", fullRepoName)
			continue
		}
		for _, p := range ps {
			if p.Name == o.jobName {
				pjs = pjutil.PresubmitSpec(p, prowapi.Refs{
					Org:     org,
					Repo:    repo,
					BaseRef: o.baseRef,
					BaseSHA: o.baseSha,
					Pulls: []prowapi.Pull{{
						Author: o.pullAuthor,
						Number: o.pullNumber,
						SHA:    o.pullSha,
					}},
				})
				labels = p.Labels
				found = true
				needsBaseRef = true
				needsPR = true
				o.org = org
				o.repo = repo
			}
		}
	}
	for fullRepoName, ps := range conf.Postsubmits {
		org, repo, err := splitRepoName(fullRepoName)
		if err != nil {
			logrus.WithError(err).Warnf("Invalid repo name %s.", fullRepoName)
			continue
		}
		for _, p := range ps {
			if p.Name == o.jobName {
				pjs = pjutil.PostsubmitSpec(p, prowapi.Refs{
					Org:     org,
					Repo:    repo,
					BaseRef: o.baseRef,
					BaseSHA: o.baseSha,
				})
				labels = p.Labels
				found = true
				needsBaseRef = true
				o.org = org
				o.repo = repo
			}
		}
	}
	for _, p := range conf.Periodics {
		if p.Name == o.jobName {
			pjs = pjutil.PeriodicSpec(p)
			labels = p.Labels
			found = true
		}
	}
	if !found {
		logrus.Fatalf("Job %s not found.", o.jobName)
	}
	if needsPR {
		if err := o.defaultPR(&pjs); err != nil {
			logrus.Fatalf("failed to default PR: %v", err)
		}
	}
	if needsBaseRef {
		if err := o.defaultBaseRef(&pjs); err != nil {
			logrus.Fatalf("failed to default base ref: %v", err)
		}
	}
	pj := pjutil.NewProwJob(pjs, labels)
	b, err := yaml.Marshal(&pj)
	if err != nil {
		logrus.WithError(err).Fatal("Error marshalling YAML.")
	}
	fmt.Print(string(b))
}

func splitRepoName(repo string) (string, string, error) {
	s := strings.SplitN(repo, "/", 2)
	if len(s) != 2 {
		return "", "", fmt.Errorf("repo %s cannot be split into org/repo", repo)
	}
	return s[0], s[1], nil
}
