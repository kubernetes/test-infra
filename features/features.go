/*
Copyright 2016 The Kubernetes Authors All rights reserved.

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

package features

import (
	"fmt"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
)

// Features are all features the code know about. Care should be taken
// not to try to use a feature which isn't 'active'
type Features struct {
	Repos  *RepoInfo
	active []feature
}

type feature interface {
	Name() string
	AddFlags(cmd *cobra.Command)
	Initialize() error
	EachLoop() error
}

var featureMap = map[string]feature{}

// GetActive returns all features requested by a munger
func (f *Features) GetActive() []feature {
	return f.active
}

// Initialize should be called with the set of all features needed by all (active) mungers
func (f *Features) Initialize(requestedFeatures []string) error {
	for _, name := range requestedFeatures {
		glog.Infof("Initilizing feature: %v", name)
		feat, found := featureMap[name]
		if !found {
			return fmt.Errorf("Could not find a feature named: %s", name)
		}
		f.active = append(f.active, featureMap[name])
		if err := feat.Initialize(); err != nil {
			return err
		}
		switch name {
		case RepoFeatureName:
			f.Repos = feat.(*RepoInfo)
		}
	}
	return nil
}

// EachLoop allows active features to update every loop
func (f *Features) EachLoop() error {
	for _, feat := range f.GetActive() {
		if err := feat.EachLoop(); err != nil {
			return err
		}
	}
	return nil
}

// AddFlags allow every feature to add flags to the command
func (f *Features) AddFlags(cmd *cobra.Command) error {
	for _, feat := range featureMap {
		feat.AddFlags(cmd)
	}
	return nil
}

// RegisterFeature should be called in `init()` by each feature to make itself
// available by name
func RegisterFeature(feat feature) error {
	if _, found := featureMap[feat.Name()]; found {
		glog.Fatalf("a feature with the name (%s) already exists", feat.Name())
	}
	featureMap[feat.Name()] = feat
	glog.Infof("Registered %#v at %s", feat, feat.Name())
	return nil
}
