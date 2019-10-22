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
	"fmt"

	"github.com/sirupsen/logrus"
)

// Publisher knows how to publish local work to a remote
type Publisher interface {
	Commit(title, body string) error
	ForcePush(branch string) error
}

type publisher struct {
	executor Executor
	remote   RemoteResolver
	info     func() (name, email string, err error)
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
		if _, err := p.executor.Run(command...); err != nil {
			return err
		}
	}
	return nil
}

// ForcePush pushes the local state to the remote
func (p *publisher) ForcePush(branch string) error {
	p.logger.Infof("Pushing branch %q", branch)
	remote, err := p.remote()
	if err != nil {
		return err
	}
	_, err = p.executor.Run("push", "--force", remote, branch)
	return err
}
