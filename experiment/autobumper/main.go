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
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/experiment/autobumper/bumper"
	"k8s.io/test-infra/prow/config/secret"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/robots/pr-creator/updater"
)

const (
	oncallAddress = "https://storage.googleapis.com/kubernetes-jenkins/oncall.json"
	githubOrg     = "kubernetes"
	githubRepo    = "test-infra"
)

var extraFiles = map[string]bool{
	"config/jobs/kubernetes/kops/build-grid.py": true,
	"releng/generate_tests.py":                  true,
	"images/kubekins-e2e/Dockerfile":            true,
}

func cdToRootDir() error {
	if bazelWorkspace := os.Getenv("BUILD_WORKSPACE_DIRECTORY"); bazelWorkspace != "" {
		if err := os.Chdir(bazelWorkspace); err != nil {
			return fmt.Errorf("failed to chdir to bazel workspace (%s): %v", bazelWorkspace, err)
		}
		return nil
	}
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return err
	}
	d := strings.TrimSpace(string(output))
	logrus.Infof("Changing working directory to %s...", d)
	return os.Chdir(d)
}

type options struct {
	githubLogin string
	githubToken string
	gitName     string
	gitEmail    string
}

func parseOptions() options {
	var o options
	flag.StringVar(&o.githubLogin, "github-login", "", "The GitHub username to use.")
	flag.StringVar(&o.githubToken, "github-token", "", "The path to the GitHub token file.")
	flag.StringVar(&o.gitName, "git-name", "", "The name to use on the git commit. Requires --git-email. If not specified, uses values from the user associated with the access token.")
	flag.StringVar(&o.gitEmail, "git-email", "", "The email to use on the git commit. Requires --git-name. If not specified, uses values from the user associated with the access token.")
	flag.Parse()
	return o
}

func validateOptions(o options) error {
	if o.githubToken == "" {
		return fmt.Errorf("--github-token is mandatory")
	}
	if (o.gitEmail == "") != (o.gitName == "") {
		return fmt.Errorf("--git-name and --git-email must be specified together")
	}
	return nil
}

func getOncaller() (string, error) {
	req, err := http.Get(oncallAddress)
	if err != nil {
		return "", err
	}
	defer req.Body.Close()
	if req.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP error %d (%q) fetching current oncaller", req.StatusCode, req.Status)
	}
	oncall := struct {
		Oncall struct {
			TestInfra string `json:"testinfra"`
		} `json:"Oncall"`
	}{}
	if err := json.NewDecoder(req.Body).Decode(&oncall); err != nil {
		return "", err
	}
	return oncall.Oncall.TestInfra, nil
}

func getAssignment() string {
	oncaller, err := getOncaller()
	if err == nil {
		if oncaller != "" {
			return "/cc @" + oncaller
		}
		return "Nobody is currently oncall, so falling back to Blunderbuss."
	}
	return fmt.Sprintf("An error occurred while finding an assignee: `%s`.\nFalling back to Blunderbuss.", err)
}

func main() {
	o := parseOptions()
	if err := validateOptions(o); err != nil {
		logrus.WithError(err).Fatal("Invalid arguments.")
	}

	sa := &secret.Agent{}
	if err := sa.Start([]string{o.githubToken}); err != nil {
		logrus.WithError(err).Fatal("Failed to start secrets agent")
	}

	gc := github.NewClient(sa.GetTokenGenerator(o.githubToken), sa.Censor, github.DefaultGraphQLEndpoint, github.DefaultAPIEndpoint)

	if o.githubLogin == "" || o.gitName == "" || o.gitEmail == "" {
		user, err := gc.BotUser()
		if err != nil {
			logrus.WithError(err).Fatal("Failed to get the user data for the provided GH token.")
		}
		if o.githubLogin == "" {
			o.githubLogin = user.Login
		}
		if o.gitName == "" {
			o.gitName = user.Name
		}
		if o.gitEmail == "" {
			o.gitEmail = user.Email
		}
	}

	stdout := bumper.HideSecretsWriter{Delegate: os.Stdout, Censor: sa}
	stderr := bumper.HideSecretsWriter{Delegate: os.Stderr, Censor: sa}

	if err := cdToRootDir(); err != nil {
		logrus.WithError(err).Fatal("Failed to change to root dir")
	}
	images, err := bumper.UpdateReferences([]string{"."}, extraFiles)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to update references.")
	}

	changed, err := bumper.HasChanges()
	if err != nil {
		logrus.WithError(err).Fatal("error occurred when checking changes")
	}

	if !changed {
		logrus.Info("no images updated, exiting ...")
		return
	}

	remoteBranch := "autobump"

	if err := bumper.MakeGitCommit(fmt.Sprintf("git@github.com:%s/test-infra.git", o.githubLogin), remoteBranch, o.gitName, o.gitEmail, images, stdout, stderr); err != nil {
		logrus.WithError(err).Fatal("Failed to push changes.")
	}

	if err := bumper.UpdatePR(gc, githubOrg, githubRepo, images, getAssignment(), "Update prow to", o.githubLogin+":"+remoteBranch, "master", updater.PreventMods); err != nil {
		logrus.WithError(err).Fatal("PR creation failed.")
	}
}
