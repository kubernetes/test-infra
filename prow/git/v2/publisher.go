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

package git

import (
	"errors"
	"fmt"

	"github.com/sirupsen/logrus"
)

// Publisher knows how to publish local work to a remote
type Publisher interface {
	// Commit stages all changes and commits them with the message
	Commit(title, body string) error
	// PushToFork pushes the local state to the fork remote
	PushToFork(branch string, force bool) error
	// PushToNamedFork is used for when the fork has a different name than the original repp
	PushToNamedFork(forkName, branch string, force bool) error
	// PushToCentral pushes the local state to the central remote
	PushToCentral(branch string, force bool) error
}

// GitUserGetter fetches a name and email for us in git commits on-demand
type GitUserGetter func() (name, email string, err error)

type remotes struct {
	publishRemote RemoteResolver
	centralRemote RemoteResolver
}

type publisher struct {
	executor executor
	remotes  remotes
	info     GitUserGetter
	logger   *logrus.Entry
}

// Commit adds all of the current content to the index and creates a commit
func (p *publisher) Commit(title, body string) error {
	p.logger.Infof("Committing changes with title %q", title)
	name, email, err := p.info()
	if err != nil {
		return err
	}
	commands := [][]string{
		{"add", "--all"},
		{"commit", "--message", title, "--message", body, "--author", fmt.Sprintf("%s <%s>", name, email)},
	}
	for _, command := range commands {
		if out, err := p.executor.Run(command...); err != nil {
			return fmt.Errorf("error committing %q: %v %v", title, err, string(out))
		}
	}
	return nil
}

func (p *publisher) PushToNamedFork(forkName, branch string, force bool) error {
	return errors.New("pushToNamedFork is not implemented in the v2 client")
}

// PublishPush pushes the local state to the publish remote
func (p *publisher) PushToFork(branch string, force bool) error {
	remote, err := p.remotes.publishRemote()
	if err != nil {
		return err
	}

	args := []string{"push"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, []string{remote, branch}...)

	p.logger.Infof("Pushing branch %q to %q", branch, remote)
	if out, err := p.executor.Run(args...); err != nil {
		return fmt.Errorf("error pushing %q: %v %v", branch, err, string(out))
	}
	return nil
}

// CentralPush pushes the local state to the central remote
func (p *publisher) PushToCentral(branch string, force bool) error {
	remote, err := p.remotes.centralRemote()
	if err != nil {
		return err
	}

	args := []string{"push"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, []string{remote, branch}...)

	p.logger.Infof("Pushing branch %q to %q", branch, remote)
	if out, err := p.executor.Run(args...); err != nil {
		return fmt.Errorf("error pushing %q: %v %v", branch, err, string(out))
	}
	return nil
}
