/*
Copyright 2022 The Kubernetes Authors.

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

package checker

import (
	"strings"

	"github.com/blang/semver"
	git "github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	gitmemory "github.com/go-git/go-git/v5/storage/memory"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// Checker is the main structure of checking if we're in Test Freeze.
type Checker struct {
	checker checker
	log     *logrus.Entry
}

// Result is the result returned by `InTestFreeze`.
type Result struct {
	// InTestFreeze is true if we're in Test Freeze.
	InTestFreeze bool

	// Branch is the found latest release branch.
	Branch string

	// Tag is the latest minor release tag to be expected.
	Tag string
}

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate
//counterfeiter:generate . checker
type checker interface {
	ListRefs(*git.Remote) ([]*plumbing.Reference, error)
}

type defaultChecker struct{}

// New creates a new Checker instance.
func New(log *logrus.Entry) *Checker {
	return &Checker{
		checker: &defaultChecker{},
		log:     log,
	}
}

// InTestFreeze returns if we're in Test Freeze:
// https://github.com/kubernetes/sig-release/blob/2d8a1cc/releases/release_phases.md#test-freeze
// It errors in case of any issue.
func (c *Checker) InTestFreeze() (*Result, error) {
	remote := git.NewRemote(gitmemory.NewStorage(), &gitconfig.RemoteConfig{
		Name: "origin",
		URLs: []string{"https://github.com/kubernetes/kubernetes"},
	})

	refs, err := c.checker.ListRefs(remote)
	if err != nil {
		c.log.Errorf("Unable to list git remote: %v", err)
		return nil, errors.Wrap(err, "list git remote")
	}

	const releaseBranchPrefix = "release-"
	var (
		latestSemver semver.Version
		latestBranch string
	)

	for _, ref := range refs {
		if ref.Name().IsBranch() {
			branch := ref.Name().Short()

			// Filter for release branches
			if !strings.HasPrefix(branch, releaseBranchPrefix) {
				continue
			}

			// Try to parse the latest minor version
			version := strings.TrimPrefix(branch, releaseBranchPrefix) + ".0"

			parsed, err := semver.Parse(version)
			if err != nil {
				c.log.Warnf("Unable parse version %s: %v", version, err)
				continue
			}

			if parsed.GT(latestSemver) {
				latestSemver = parsed
				latestBranch = branch
			}
		}
	}

	if latestBranch == "" {
		return nil, errors.New("no latest release branch found")
	}

	for _, ref := range refs {
		if ref.Name().IsTag() {
			tag := strings.TrimPrefix(ref.Name().Short(), "v")

			parsed, err := semver.Parse(tag)
			if err != nil {
				c.log.Warnf("Unable parse tag %s: %v", tag, err)
				continue
			}

			// Found the latest minor version on the latest release branch,
			// which means we're not in Test Freeze.
			if latestSemver.EQ(parsed) {
				return &Result{
					InTestFreeze: false,
					Branch:       latestBranch,
					Tag:          "v" + tag,
				}, nil
			}
		}
	}

	// Latest minor version not found in latest release branch,
	// we're in Test Freeze.
	return &Result{
		InTestFreeze: true,
		Branch:       latestBranch,
		Tag:          "v" + latestSemver.String(),
	}, nil
}

func (*defaultChecker) ListRefs(r *git.Remote) ([]*plumbing.Reference, error) {
	return r.List(&git.ListOptions{})
}
