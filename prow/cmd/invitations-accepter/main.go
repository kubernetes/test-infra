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
	ListCurrentUserOrgInvitations() ([]github.UserOrgInvitation, error)
	AcceptUserOrgInvitation(org string) error
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

	logger := logrus.WithField("bot-user", botUser.Login)

	if err := acceptOrgInvitations(gc, dryRun, logger); err != nil {
		errs = append(errs, err)
	}

	if err := acceptRepoInvitations(gc, dryRun, logger); err != nil {
		errs = append(errs, err)
	}

	return utilerrors.NewAggregate(errs)
}

func acceptOrgInvitations(gc githubClient, dryRun bool, logger *logrus.Entry) error {
	var errs []error

	orgInvitations, err := gc.ListCurrentUserOrgInvitations()
	if err != nil {
		return fmt.Errorf("couldn't get org invitations for the authenticated user: %v", err)
	}

	for _, inv := range orgInvitations {
		org := inv.Org.Login
		orgLogger := logger.WithField("org", org)
		if dryRun {
			orgLogger.Info("(dry-run) Accepting organization invitation.")
		} else {
			orgLogger.Info("Accepting organization invitation.")
			errs = append(errs, gc.AcceptUserOrgInvitation(org))
		}
	}
	return utilerrors.NewAggregate(errs)

}

func acceptRepoInvitations(gc githubClient, dryRun bool, logger *logrus.Entry) error {
	var errs []error

	repoInvitations, err := gc.ListCurrentUserRepoInvitations()
	if err != nil {
		return fmt.Errorf("couldn't get repo invitations for the authenticated user: %v", err)
	}

	for _, inv := range repoInvitations {
		repoLogger := logger.WithFields(logrus.Fields{"invitation-id": inv.InvitationID, "repo": inv.Repository.FullName})
		if dryRun {
			repoLogger.Info("(dry-run) Accepting repository invitation.")
		} else {
			repoLogger.Info("Accepting repository invitation.")
			errs = append(errs, gc.AcceptUserRepoInvitation(inv.InvitationID))
		}
	}
	return utilerrors.NewAggregate(errs)
}
