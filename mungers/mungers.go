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

	"k8s.io/contrib/mungegithub/features"
	"k8s.io/contrib/mungegithub/github"
	"k8s.io/kubernetes/pkg/util/sets"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
)

// Munger is the interface which all mungers must implement to register
type Munger interface {
	// Take action on a specific github issue:
	Munge(obj *github.MungeObject)
	AddFlags(cmd *cobra.Command, config *github.Config)
	Name() string
	RequiredFeatures() []string
	Initialize(*github.Config, *features.Features) error
	EachLoop() error
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

// RequestedFeatures returns a list of all feature which should be enabled
// for the running mungers
func RequestedFeatures() []string {
	out := sets.NewString()
	for _, m := range GetActiveMungers() {
		f := m.RequiredFeatures()
		out.Insert(f...)
	}
	return out.List()
}

// RegisterMungers will check if a requested munger exists and add it to
// the list.
func RegisterMungers(requestedMungers []string) error {
	for _, name := range requestedMungers {
		munger, found := mungerMap[name]
		if !found {
			return fmt.Errorf("couldn't find a munger named: %s", name)
		}
		mungers = append(mungers, munger)
	}
	return nil
}

// InitializeMungers will call munger.Initialize() for the requested mungers.
func InitializeMungers(config *github.Config, features *features.Features) error {
	for _, munger := range mungers {
		if err := munger.Initialize(config, features); err != nil {
			return err
		}
		glog.Infof("Initialized munger: %s", munger.Name())
	}
	return nil
}

// EachLoop will be called before we start a poll loop and will run the
// EachLoop function for all active mungers
func EachLoop() error {
	for _, munger := range mungers {
		if err := munger.EachLoop(); err != nil {
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

// MungeIssue will call each activated munger with the given object
func MungeIssue(obj *github.MungeObject) error {
	for _, munger := range mungers {
		munger.Munge(obj)
	}
	return nil
}
