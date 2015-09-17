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

	"k8s.io/contrib/mungegithub/config"

	"github.com/golang/glog"
	github_api "github.com/google/go-github/github"
)

var mungerMap = map[string]config.IssueMunger{}

func GetAllMungers() []config.IssueMunger {
	out := []config.IssueMunger{}
	for _, munger := range mungerMap {
		out = append(out, munger)
	}
	return out
}

func getMungers(mungers []string) ([]config.IssueMunger, error) {
	result := make([]config.IssueMunger, len(mungers))
	for ix := range mungers {
		munger, found := mungerMap[mungers[ix]]
		if !found {
			return nil, fmt.Errorf("Couldn't find a munger named: %s", mungers[ix])
		}
		result[ix] = munger
	}
	return result, nil
}

func RegisterMunger(munger config.IssueMunger) error {
	if _, found := mungerMap[munger.Name()]; found {
		return fmt.Errorf("A munger with that name (%s) already exists", munger.Name())
	}
	mungerMap[munger.Name()] = munger
	glog.Infof("Registered %#v at %s", munger, munger.Name())
	return nil
}

func mungeIssue(config *config.MungeConfig, issue *github_api.Issue) error {
	for _, munger := range config.IssueMungers {
		munger.MungeIssue(config, issue)
	}
	return nil
}

func MungeIssues(config *config.MungeConfig) error {
	mfunc := func(issue *github_api.Issue) error {
		return mungeIssue(config, issue)
	}
	if err := config.ForEachIssueDo([]string{}, mfunc); err != nil {
		return err
	}
	return nil
}
