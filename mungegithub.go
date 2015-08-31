/*
Copyright 2015 The Kubernetes Authors All rights reserved.

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
	"io/ioutil"

	"k8s.io/contrib/mungegithub/issues"
	"k8s.io/contrib/mungegithub/opts"
	"k8s.io/contrib/mungegithub/pulls"
	"k8s.io/contrib/submit-queue/github"

	"github.com/golang/glog"
)

var (
	o            = opts.MungeOptions{}
	token        = flag.String("token", "", "The OAuth Token to use for requests.")
	tokenFile    = flag.String("token-file", "", "A file containing the OAUTH token to use for requests.")
	issueMungers = flag.String("issue-mungers", "", "A list of issue mungers to run")
	prMungers    = flag.String("pr-mungers", "", "A list of pull request mungers to run")
)

func init() {
	flag.IntVar(&o.MinPRNumber, "min-pr-number", 0, "The minimum PR to start with [default: 0]")
	flag.IntVar(&o.MinIssueNumber, "min-issue-number", 0, "The minimum PR to start with [default: 0]")
	flag.BoolVar(&o.Dryrun, "dry-run", false, "If true, don't actually merge anything")
	flag.StringVar(&o.Org, "organization", "kubernetes", "The github organization to scan")
	flag.StringVar(&o.Project, "project", "kubernetes", "The github project to scan")
}

func main() {
	flag.Parse()
	if len(o.Org) == 0 {
		glog.Fatalf("--organization is required.")
	}
	if len(o.Project) == 0 {
		glog.Fatalf("--project is required.")
	}
	tokenData := *token
	if len(tokenData) == 0 && len(*tokenFile) != 0 {
		data, err := ioutil.ReadFile(*tokenFile)
		if err != nil {
			glog.Fatalf("error reading token file: %v", err)
		}
		tokenData = string(data)
	}
	client := github.MakeClient(tokenData)

	if len(*issueMungers) > 0 {
		glog.Infof("Running issue mungers")
		if err := issues.MungeIssues(client, *issueMungers, o); err != nil {
			glog.Errorf("Error munging issues: %v", err)
		}
	}
	if len(*prMungers) > 0 {
		glog.Infof("Running PR mungers")
		if err := pulls.MungePullRequests(client, *prMungers, o); err != nil {
			glog.Errorf("Error munging PRs: %v", err)
		}
	}
}
