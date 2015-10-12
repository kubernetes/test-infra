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

package mungers

import (
	"fmt"

	"k8s.io/contrib/mungegithub/github"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
)

// Munger is the interface which all mungers must implement to register
type Munger interface {
	// Take action on a specific github issue:
	MungePullRequest(obj *github.MungeObject)
	AddFlags(cmd *cobra.Command, config *github.Config)
	Name() string
	Initialize(*github.Config) error
	EachLoop(*github.Config) error
}

var mungerMap = map[string]Munger{}
var mungers = []Munger{}

// GetAllMungers returns a slice of all registered mungers. This list is
// completely independant of the mungers selected at runtime in --pr-mungers.
// This is all possible mungers.
func GetAllMungers() []Munger {
	out := []Munger{}
	for _, munger := range mungerMap {
		out = append(out, munger)
	}
	return out
}

// GetActiveMungers returns a slice of all mungers which both registered and
// were requested by the user
func GetActiveMungers() []Munger {
	return mungers
}

// InitializeMungers will call munger.Initialize() for all mungers requested
// in --pr-mungers
func InitializeMungers(requestedMungers []string, config *github.Config) error {
	for _, name := range requestedMungers {
		munger, found := mungerMap[name]
		if !found {
			return fmt.Errorf("couldn't find a munger named: %s", name)
		}
		mungers = append(mungers, munger)
		if err := munger.Initialize(config); err != nil {
			return err
		}
	}
	return nil
}

// EachLoop will be called before we start a poll loop and will run the
// EachLoop function for all active mungers
func EachLoop(config *github.Config) error {
	for _, munger := range mungers {
		if err := munger.EachLoop(config); err != nil {
			return err
		}
	}
	return nil
}

// RegisterMunger should be called in `init()` by each munger to make itself
// available by name
func RegisterMunger(munger Munger) error {
	if _, found := mungerMap[munger.Name()]; found {
		return fmt.Errorf("a munger with that name (%s) already exists", munger.Name())
	}
	mungerMap[munger.Name()] = munger
	glog.Infof("Registered %#v at %s", munger, munger.Name())
	return nil
}

// RegisterMungerOrDie will call RegisterMunger but will be fatal on error
func RegisterMungerOrDie(munger Munger) {
	if err := RegisterMunger(munger); err != nil {
		glog.Fatalf("Failed to register munger: %s", err)
	}
}

func MungeIssue(obj *github.MungeObject) error {
	// This really belongs in the individual munger
	if !obj.IsPR() {
		return nil
	}

	_, err := obj.GetPR()
	if err != nil {
		return err
	}

	merged, err := obj.IsMerged()
	if err != nil {
		return err
	}
	if merged {
		glog.V(3).Infof("PR %d was merged, may want to reduce the PerPage so this happens less often", *obj.Issue.Number)
		return nil
	}

	_, err = obj.GetCommits()
	if err != nil {
		return err
	}

	_, err = obj.GetEvents()
	if err != nil {
		return err
	}

	for _, munger := range mungers {
		munger.MungePullRequest(obj)
	}
	return nil
}
