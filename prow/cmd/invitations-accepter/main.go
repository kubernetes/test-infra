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
	"fmt"
	"os"

	"github.com/sirupsen/logrus"

	utilerrors "k8s.io/apimachinery/pkg/util/errors"

	"k8s.io/test-infra/prow/config/secret"
	"k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/github"
)

const (
	defaultTokens = 300
	defaultBurst  = 300
)

type options struct {
	github flagutil.GitHubOptions

	dryRun bool
}

type githubClient interface {
	ListCurrentUserRepoInvitations() ([]github.UserRepoInvitation, error)
	AcceptUserRepoInvitation(invitationID int) error
	BotUser() (*github.UserData, error)
}

func gatherOptions() options {
	o := options{}
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	fs.BoolVar(&o.dryRun, "dry-run", true, "Dry run for testing. Uses API tokens but does not mutate.")

	o.github.AddCustomizedFlags(fs, flagutil.ThrottlerDefaults(defaultTokens, defaultBurst))
	if err := fs.Parse(os.Args[1:]); err != nil {
		logrus.WithError(err).Fatal("could not parse input")
	}
	return o
}

func (o *options) Validate() error {
	return o.github.Validate(o.dryRun)
}

func main() {
	o := gatherOptions()

	sa := &secret.Agent{}
	if err := sa.Start(nil); err != nil {
		logrus.WithError(err).Fatal("Error starting secrets agent.")
	}

	gc, err := o.github.GitHubClient(sa, o.dryRun)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting GitHub client.")
	}

	if err := acceptInvitations(gc, o.dryRun); err != nil {
		logrus.WithError(err).Fatal("Errors occurred.")
	}
}

func acceptInvitations(gc githubClient, dryRun bool) error {
	var errs []error

	botUser, err := gc.BotUser()
	if err != nil {
		return fmt.Errorf("couldn't get bot's user name: %v", err)
	}
	repoInvitations, err := gc.ListCurrentUserRepoInvitations()
	if err != nil {
		return fmt.Errorf("couldn't get repo invitations for the authenticated user: %v", err)
	}

	for _, inv := range repoInvitations {
		logger := logrus.WithFields(logrus.Fields{"bot-user": botUser.Login, "invitation-id": inv.InvitationID, "repo": inv.Repository.FullName})
		if dryRun {
			logger.Info("(dry-run) Accepting invitation.")
		} else {
			logger.Info("Accepting invitation.")
			errs = append(errs, gc.AcceptUserRepoInvitation(inv.InvitationID))
		}
	}

	return utilerrors.NewAggregate(errs)
}
