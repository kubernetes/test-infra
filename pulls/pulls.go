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

package pulls

import (
	"fmt"

	"k8s.io/contrib/mungegithub/config"

	"github.com/golang/glog"
	github_api "github.com/google/go-github/github"
)

var mungerMap = map[string]config.PRMunger{}

func GetAllMungers() []config.PRMunger {
	out := []config.PRMunger{}
	for _, munger := range mungerMap {
		out = append(out, munger)
	}
	return out
}

func getMungers(mungers []string) ([]config.PRMunger, error) {
	result := make([]config.PRMunger, len(mungers))
	for ix := range mungers {
		munger, found := mungerMap[mungers[ix]]
		if !found {
			return nil, fmt.Errorf("Couldn't find a munger named: %s", mungers[ix])
		}
		result[ix] = munger
	}
	return result, nil
}

func RegisterMunger(munger config.PRMunger) error {
	if _, found := mungerMap[munger.Name()]; found {
		return fmt.Errorf("A munger with that name (%s) already exists", munger.Name())
	}
	mungerMap[munger.Name()] = munger
	glog.Infof("Registered %#v at %s", munger, munger.Name())
	return nil
}

func RegisterMungerOrDie(munger config.PRMunger) {
	if err := RegisterMunger(munger); err != nil {
		glog.Fatalf("Failed to register munger: %s", err)
	}
}

func mungePR(config *config.MungeConfig, pr *github_api.PullRequest, issue *github_api.Issue) error {
	if pr == nil {
		fmt.Printf("found nil pr\n")
	}
	mungers, err := getMungers(config.PRMungersList)
	if err != nil {
		return err
	}

	commits, err := config.GetFilledCommits(*pr.Number)
	if err != nil {
		return err
	}

	events, err := config.GetAllEventsForPR(*pr.Number)
	if err != nil {
		return err
	}

	for _, munger := range mungers {
		munger.MungePullRequest(config, pr, issue, commits, events)
	}
	return nil
}

func MungePullRequests(config *config.MungeConfig) error {
	mfunc := func(pr *github_api.PullRequest, issue *github_api.Issue) error {
		return mungePR(config, pr, issue)
	}
	if err := config.ForEachPRDo([]string{}, mfunc); err != nil {
		return err
	}

	return nil
}
