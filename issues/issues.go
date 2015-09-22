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

package issues

import (
	"fmt"

	github_util "k8s.io/contrib/mungegithub/github"

	"github.com/golang/glog"
	github_api "github.com/google/go-github/github"
	"github.com/spf13/cobra"
)

// IssueMunger is the interface which all mungers must implement to register
type IssueMunger interface {
	MungeIssue(config *github_util.Config, issue *github_api.Issue)
	AddFlags(cmd *cobra.Command, config *github_util.Config)
	Initialize(*github_util.Config) error
	Name() string
}

var (
	mungerMap = map[string]IssueMunger{}
	mungers   = []IssueMunger{}
)

// GetAllMungers returns a slice of all registered mungers. This list is
// completely independant of the mungers selected at runtime in --pr-mungers.
// This is all possible mungers.
func GetAllMungers() []IssueMunger {
	out := []IssueMunger{}
	for _, munger := range mungerMap {
		out = append(out, munger)
	}
	return out
}

// InitializeMungers will call munger.Initialize() for all mungers
// requested in --issue-mungers
func InitializeMungers(requestedMungers []string, config *github_util.Config) error {
	for _, name := range requestedMungers {
		munger, found := mungerMap[name]
		if !found {
			return fmt.Errorf("couldn't find a munger named: %s", name)
		}
		if err := munger.Initialize(config); err != nil {
			return err
		}
		mungers = append(mungers, munger)
	}
	return nil
}

// RegisterMunger should be called in `init()` by each munger to make itself
// available by name
func RegisterMunger(munger IssueMunger) error {
	if _, found := mungerMap[munger.Name()]; found {
		return fmt.Errorf("a munger with that name (%s) already exists", munger.Name())
	}
	mungerMap[munger.Name()] = munger
	glog.Infof("Registered %#v at %s", munger, munger.Name())
	return nil
}

func mungeIssue(config *github_util.Config, issue *github_api.Issue) error {
	for _, munger := range mungers {
		munger.MungeIssue(config, issue)
	}
	return nil
}

// MungeIssues is the main function which asks that each munger be called
// for each Issue
func MungeIssues(config *github_util.Config) error {
	mfunc := func(issue *github_api.Issue) error {
		return mungeIssue(config, issue)
	}
	if err := config.ForEachIssueDo([]string{}, mfunc); err != nil {
		return err
	}
	return nil
}
