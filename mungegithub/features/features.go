/*
Copyright 2016 The Kubernetes Authors.

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

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/mungegithub/github"
	"k8s.io/test-infra/mungegithub/options"

	"github.com/golang/glog"
)

// Features are all features the code know about. Care should be taken
// not to try to use a feature which isn't 'active'
type Features struct {
	Repos  *RepoInfo
	Server *ServerFeature
	active []feature
}

type feature interface {
	Name() string
	RegisterOptions(opts *options.Options) sets.String
	Initialize(config *github.Config) error
	EachLoop() error
}

var featureMap = map[string]feature{}

// GetActive returns all features requested by a munger
func (f *Features) GetActive() []feature {
	return f.active
}

// Initialize should be called with the set of all features needed by all (active) mungers
func (f *Features) Initialize(config *github.Config, requestedFeatures []string) error {
	for _, name := range requestedFeatures {
		glog.Infof("Initializing feature: %v", name)
		feat, found := featureMap[name]
		if !found {
			return fmt.Errorf("Could not find a feature named: %s", name)
		}
		f.active = append(f.active, featureMap[name])
		if err := feat.Initialize(config); err != nil {
			return err
		}
		switch name {
		case RepoFeatureName:
			f.Repos = feat.(*RepoInfo)
		case ServerFeatureName:
			f.Server = feat.(*ServerFeature)
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

// RegisterOptions registers the options used by features and returns any options that should
// trigger a restart when they are changed.
func (f *Features) RegisterOptions(opts *options.Options) sets.String {
	immutables := sets.NewString()
	for _, feat := range featureMap {
		immutables = immutables.Union(feat.RegisterOptions(opts))
	}
	return immutables
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
