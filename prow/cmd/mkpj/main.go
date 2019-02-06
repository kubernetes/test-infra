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

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/kube"
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
	flag.StringVar(&o.jobName, "job", "", "Job to run.")
	flag.StringVar(&o.configPath, "config-path", "", "Path to config.yaml.")
	flag.StringVar(&o.jobConfigPath, "job-config-path", "", "Path to prow job configs.")
	flag.StringVar(&o.baseRef, "base-ref", "", "Git base ref under test")
	flag.StringVar(&o.baseSha, "base-sha", "", "Git base SHA under test")
	flag.IntVar(&o.pullNumber, "pull-number", 0, "Git pull number under test")
	flag.StringVar(&o.pullSha, "pull-sha", "", "Git pull SHA under test")
	flag.StringVar(&o.pullAuthor, "pull-author", "", "Git pull author under test")
	flag.Parse()
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

	var pjs kube.ProwJobSpec
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
				pjs = pjutil.PresubmitSpec(p, kube.Refs{
					Org:     org,
					Repo:    repo,
					BaseRef: o.baseRef,
					BaseSHA: o.baseSha,
					Pulls: []kube.Pull{{
						Author: o.pullAuthor,
						Number: o.pullNumber,
						SHA:    o.pullSha,
					}},
				})
				labels = p.Labels
				found = true
				needsBaseRef = true
				needsPR = true
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
				pjs = pjutil.PostsubmitSpec(p, kube.Refs{
					Org:     org,
					Repo:    repo,
					BaseRef: o.baseRef,
					BaseSHA: o.baseSha,
				})
				labels = p.Labels
				found = true
				needsBaseRef = true
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
	if needsBaseRef {
		if pjs.Refs.BaseRef == "" {
			fmt.Fprint(os.Stderr, "Base ref (e.g. master): ")
			fmt.Scanln(&pjs.Refs.BaseRef)
		}
		if pjs.Refs.BaseSHA == "" {
			fmt.Fprint(os.Stderr, "Base SHA (e.g. 72bcb5d80): ")
			fmt.Scanln(&pjs.Refs.BaseSHA)
		}
	}
	if needsPR {
		if pjs.Refs.Pulls[0].Number == 0 {
			fmt.Fprint(os.Stderr, "PR Number: ")
			fmt.Scanln(&pjs.Refs.Pulls[0].Number)
		}
		if pjs.Refs.Pulls[0].Author == "" {
			fmt.Fprint(os.Stderr, "PR author: ")
			fmt.Scanln(&pjs.Refs.Pulls[0].Author)
		}
		if pjs.Refs.Pulls[0].SHA == "" {
			fmt.Fprint(os.Stderr, "PR SHA (e.g. 72bcb5d80): ")
			fmt.Scanln(&pjs.Refs.Pulls[0].SHA)
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
