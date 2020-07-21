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
	"fmt"
	"os"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/experiment/autobumper/bumper"
	"k8s.io/test-infra/prow/config/secret"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/robots/pr-creator/updater"
)

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
