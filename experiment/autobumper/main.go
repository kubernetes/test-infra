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

package main

import (
	"flag"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/experiment/autobumper/bumper"
)

func parseOptions() *bumper.Options {
	var o bumper.Options
	flag.StringVar(&o.GitHubToken, "github-token", "", "The path to the GitHub token file.")
	flag.StringVar(&o.GitHubLogin, "github-login", "", "The GitHub username to use. If not specified, uses values from the user associated with the access token.")
	flag.StringVar(&o.GitName, "git-name", "", "The name to use on the git commit. Requires --git-email. If not specified, uses values from the user associated with the access token.")
	flag.StringVar(&o.GitEmail, "git-email", "", "The email to use on the git commit. Requires --git-name. If not specified, uses values from the user associated with the access token.")
	flag.StringVar(&o.GitHubOrg, "github-org", "", "The GitHub org name where the autobump PR will be created. Must not be empty when --create-pull-request is not false.")
	flag.StringVar(&o.GitHubRepo, "github-repo", "", "The GitHub repo name where the autobump PR will be created. Must not be empty when --create-pull-request is not false.")
	flag.StringVar(&o.RemoteBranch, "remote-branch", "autobump", "The remote branch name where the files will be updated. Must not be empty when --create-pull-request is not false.")
	flag.StringVar(&o.OncallAddress, "oncall-address", "", "The oncall address where we can get the JSON file that stores the current oncall information.")

	flag.BoolVar(&o.BumpProwImages, "bump-prow-images", false, "Whether to bump up version of images in gcr.io/k8s-prow/.")
	flag.BoolVar(&o.BumpBoskosImages, "bump-boskos-images", false, "Whether to bump up version of images in gcr.io/k8s-staging-boskos/.")
	flag.BoolVar(&o.BumpTestImages, "bump-test-images", false, "Whether to bump up version of images in gcr.io/k8s-testimages/.")
	flag.StringVar(&o.TargetVersion, "target-version", "", "The target version to bump images version to, which can be one of latest, upstream, upstream-staging and vYYYYMMDD-deadbeef.")

	flag.Var(&o.IncludedConfigPaths, "include-config-paths", "The config paths to be included in this bump, in which only .yaml files will be considered. By default all files are included.")
	flag.Var(&o.ExcludedConfigPaths, "exclude-config-paths", "The config paths to be excluded in this bump, in which only .yaml files will be considered.")
	flag.Var(&o.ExtraFiles, "extra-files", "The extra non-yaml file to be considered in this bump.")

	flag.BoolVar(&o.SkipPullRequest, "skip-pull-request", false, "Whether to skip creating the pull request for this bump.")
	flag.Parse()
	return &o
}

func main() {
	o := parseOptions()

	if err := bumper.Run(o); err != nil {
		logrus.WithError(err).Fatalf("failed to run the bumper tool")
	}
}
