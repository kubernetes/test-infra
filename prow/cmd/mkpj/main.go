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

	"github.com/sirupsen/logrus"
	"sigs.k8s.io/yaml"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	prowflagutil "k8s.io/test-infra/prow/flagutil"
	configflagutil "k8s.io/test-infra/prow/flagutil/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pjutil"
)

type options struct {
	jobName     string
	config      configflagutil.ConfigOptions
	triggerJob  bool
	failWithJob bool
	kubeOptions prowflagutil.KubernetesOptions
	baseRef     string
	baseSha     string
	pullNumber  int
	pullSha     string
	pullAuthor  string
	pullHeadRef string
	org         string
	repo        string

	github       prowflagutil.GitHubOptions
	githubClient githubClient
	pullRequest  *github.PullRequest
}

func (o *options) genJobSpec(conf *config.Config) (config.JobBase, prowapi.ProwJobSpec) {
	for fullRepoName, ps := range conf.PresubmitsStatic {
		org, repo, err := config.SplitRepoName(fullRepoName)
		if err != nil {
			logrus.WithError(err).Warnf("Invalid repo name %s.", fullRepoName)
			continue
		}
		for _, p := range ps {
			if p.Name == o.jobName {
				return p.JobBase, pjutil.PresubmitSpec(p, prowapi.Refs{
					Org:     org,
					Repo:    repo,
					BaseRef: o.baseRef,
					BaseSHA: o.baseSha,
					Pulls: []prowapi.Pull{{
						Author:  o.pullAuthor,
						Number:  o.pullNumber,
						SHA:     o.pullSha,
						HeadRef: o.pullHeadRef,
					}},
				})
			}
		}
	}
	for fullRepoName, ps := range conf.PostsubmitsStatic {
		org, repo, err := config.SplitRepoName(fullRepoName)
		if err != nil {
			logrus.WithError(err).Warnf("Invalid repo name %s.", fullRepoName)
			continue
		}
		for _, p := range ps {
			if p.Name == o.jobName {
				return p.JobBase, pjutil.PostsubmitSpec(p, prowapi.Refs{
					Org:     org,
					Repo:    repo,
					BaseRef: o.baseRef,
					BaseSHA: o.baseSha,
				})
			}
		}
	}
	for _, p := range conf.Periodics {
		if p.Name == o.jobName {
			return p.JobBase, pjutil.PeriodicSpec(p)
		}
	}
	return config.JobBase{}, prowapi.ProwJobSpec{}
}

func (o *options) getPullRequest() (*github.PullRequest, error) {
	if o.pullRequest != nil {
		return o.pullRequest, nil
	}
	pr, err := o.githubClient.GetPullRequest(o.org, o.repo, o.pullNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch PullRequest from GitHub: %w", err)
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

	if err := o.config.Validate(false); err != nil {
		return err
	}

	if err := o.github.Validate(false); err != nil {
		return err
	}

	if o.triggerJob {
		if err := o.kubeOptions.Validate(false); err != nil {
			return err
		}
	}

	return nil
}

func gatherOptions() options {
	var o options
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	fs.StringVar(&o.jobName, "job", "", "Job to run.")
	fs.StringVar(&o.baseRef, "base-ref", "", "Git base ref under test")
	fs.StringVar(&o.baseSha, "base-sha", "", "Git base SHA under test")
	fs.IntVar(&o.pullNumber, "pull-number", 0, "Git pull number under test")
	fs.StringVar(&o.pullSha, "pull-sha", "", "Git pull SHA under test")
	fs.StringVar(&o.pullAuthor, "pull-author", "", "Git pull author under test")
	fs.StringVar(&o.pullHeadRef, "pull-head-ref", "", "Git branch name of the proposed change")
	fs.BoolVar(&o.triggerJob, "trigger-job", false, "Submit the job to Prow and wait for results")
	fs.BoolVar(&o.failWithJob, "fail-with-job", false, "Exit with a non-zero exit code if the triggered job fails")
	o.config.AddFlags(fs)
	o.kubeOptions.AddFlags(fs)
	o.github.AddFlags(fs)
	o.github.AllowAnonymous = true
	o.github.AllowDirectAccess = true
	fs.Parse(os.Args[1:])
	return o
}

func main() {
	o := gatherOptions()
	if err := o.Validate(); err != nil {
		logrus.WithError(err).Fatalf("Bad flags")
	}

	ca, err := o.config.ConfigAgent()
	if err != nil {
		logrus.WithError(err).Fatal("Error loading config")
	}
	conf := ca.Config()

	o.githubClient, err = o.github.GitHubClient(false)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to get GitHub client")
	}
	job, pjs := o.genJobSpec(conf)
	if job.Name == "" {
		logrus.Fatalf("Job %s not found.", o.jobName)
	}
	if pjs.Refs != nil {
		o.org = pjs.Refs.Org
		o.repo = pjs.Refs.Repo
		if len(pjs.Refs.Pulls) != 0 {
			if err := o.defaultPR(&pjs); err != nil {
				logrus.WithError(err).Fatal("Failed to default PR")
			}
		}
		if err := o.defaultBaseRef(&pjs); err != nil {
			logrus.WithError(err).Fatal("Failed to default base ref")
		}
	}
	pj := pjutil.NewProwJob(pjs, job.Labels, job.Annotations, pjutil.RequireScheduling(conf.Scheduler.Enabled))
	if !o.triggerJob {
		b, err := yaml.Marshal(&pj)
		if err != nil {
			logrus.WithError(err).Fatal("Error marshalling YAML.")
		}
		fmt.Print(string(b))
		return
	}

	if succeeded, err := pjutil.TriggerAndWatchProwJob(o.kubeOptions, &pj, conf, nil, false); err != nil {
		logrus.WithError(err).Fatalf("failed while submitting job or watching its result")
	} else if !succeeded && o.failWithJob {
		os.Exit(1)
	}
}
