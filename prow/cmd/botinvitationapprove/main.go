/*
Copyright 2021 The Kubernetes Authors.

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
	"log"
	"strings"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
	"k8s.io/test-infra/prow/config/secret"
	"k8s.io/test-infra/prow/github"

	"k8s.io/test-infra/prow/config"
)

type githubClient interface {
	ListCurrentUserInvitations() ([]github.UserInvitation, error)
	AcceptUserInvitation(invitationID int) error
	DeclineUserInvitation(invitationID int) error
}

func loadProwConfig(fp string) (*config.ProwConfig, error) {
	var pc config.ProwConfig
	content, err := ioutil.ReadFile(fp)
	if err != nil {
		return nil, err
	}
	if err = yaml.Unmarshal(content, &pc); err != nil {
		return nil, err
	}
	return &pc, nil
}

func isTrusted(orgrepo string, pc *config.ProwConfig) bool {
	org := strings.SplitN(orgrepo, "/", 2)[0]
	for trusted := range pc.ManagedWebhooks.OrgRepoConfig {
		if trusted == orgrepo {
			return true
		}
		if trusted == org {
			return true
		}
	}
	return false
}

func handle(ghc githubClient, pc *config.ProwConfig) error {
	ivs, err := ghc.ListCurrentUserInvitations()
	if err != nil {
		return err
	}
	for _, iv := range ivs {
		logrus.Infof("Processing invitation from %s", iv.Repository.FullName)
		var isNotValid bool
		if !isTrusted(iv.Repository.FullName, pc) {
			isNotValid = true
			logrus.Infof("Processing invitation from %s, the repo hasn't onboarded prow yet, need to be added under 'managed_hmac'", iv.Repository.FullName)
		} else if iv.Permission != github.Admin {
			isNotValid = true
			logrus.Infof("Processing invitation from %s, expecting admin, got %v", iv.Repository.FullName, iv.Permission)
		}

		if isNotValid {
			logrus.Infof("Rejecting invitation from %s", iv.Repository.FullName)
			if newe := ghc.DeclineUserInvitation(iv.InvitationID); newe != nil {
				logrus.WithError(newe).Warnf("Failed rejecting invitation from %s", iv.Repository.FullName)
				err = newe
			}
		} else {
			logrus.Infof("Accepting invitation from %s as admin", iv.Repository.FullName)
			if newe := ghc.AcceptUserInvitation(iv.InvitationID); newe != nil {
				logrus.WithError(newe).Warnf("Failed accepting invitation from %s", iv.Repository.FullName)
				err = newe
			}
		}
	}
	return err
}

func main() {
	githubToken := flag.String("github-token", "", "Path to github token.")
	configPath := flag.String("config-file", "", "Prow config file path for parsing trusted repo")
	flag.Parse()

	var sa secret.Agent
	if err := sa.Start([]string{*githubToken}); err != nil {
		log.Fatalf("Failed to start secrets agent: %v", err)
	}

	ghc := github.NewClient(sa.GetTokenGenerator(*githubToken), sa.Censor, github.DefaultGraphQLEndpoint, github.DefaultAPIEndpoint)

	pc, err := loadProwConfig(*configPath)
	if err != nil {
		log.Fatal(err)
	}

	if err := handle(ghc, pc); err != nil {
		log.Fatal("Failed: check error logs above for details.")
	}
}
