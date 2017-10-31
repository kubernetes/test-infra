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
	"fmt"
	"os"
	"strings"

	"github.com/ghodss/yaml"
	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pjutil"
)

var (
	jobName    = flag.String("job", "", "Job to run.")
	configPath = flag.String("config-path", "", "Path to config.yaml.")
)

func main() {
	flag.Parse()

	if *jobName == "" {
		logrus.Fatal("Must specify --job.")
	}

	conf, err := config.Load(*configPath)
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
			logrus.WithError(err).Fatal("Invalid repo name.")
		}
		for _, p := range ps {
			if p.Name == *jobName {
				pjs = pjutil.PresubmitSpec(p, kube.Refs{
					Org:   org,
					Repo:  repo,
					Pulls: []kube.Pull{{}},
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
			logrus.WithError(err).Fatal("Invalid repo name.")
		}
		for _, p := range ps {
			if p.Name == *jobName {
				pjs = pjutil.PostsubmitSpec(p, kube.Refs{
					Org:  org,
					Repo: repo,
				})
				labels = p.Labels
				found = true
				needsBaseRef = true
			}
		}
	}
	for _, p := range conf.Periodics {
		if p.Name == *jobName {
			pjs = pjutil.PeriodicSpec(p)
			labels = p.Labels
			found = true
		}
	}
	if !found {
		logrus.Fatalf("Job %s not found.", *jobName)
	}
	if needsBaseRef {
		fmt.Fprint(os.Stderr, "Base ref (e.g. master): ")
		fmt.Scanln(&pjs.Refs.BaseRef)
		fmt.Fprint(os.Stderr, "Base SHA (e.g. 72bcb5d80): ")
		fmt.Scanln(&pjs.Refs.BaseSHA)
	}
	if needsPR {
		fmt.Fprint(os.Stderr, "PR Number: ")
		fmt.Scanln(&pjs.Refs.Pulls[0].Number)
		fmt.Fprint(os.Stderr, "PR author: ")
		fmt.Scanln(&pjs.Refs.Pulls[0].Author)
		fmt.Fprint(os.Stderr, "PR SHA (e.g. 72bcb5d80): ")
		fmt.Scanln(&pjs.Refs.Pulls[0].SHA)
	}
	pj := pjutil.NewProwJob(pjs, labels)
	b, err := yaml.Marshal(&pj)
	if err != nil {
		logrus.WithError(err).Fatal("Error marshalling YAML.")
	}
	fmt.Print(string(b))
}

func splitRepoName(repo string) (string, string, error) {
	s := strings.Split(repo, "/")
	if len(s) != 2 {
		return "", "", fmt.Errorf("repo %s cannot be split into org/repo", repo)
	}
	return s[0], s[1], nil
}
