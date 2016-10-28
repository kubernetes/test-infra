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

package main

import (
	"io/ioutil"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/ghodss/yaml"
)

// TODO(spxtr): Consider better architectures for this that lets us test that
// the yaml is valid.

type PluginAgent struct {
	mut sync.Mutex
	// Repo FullName (eg "kubernetes/kubernetes") -> list of plugins
	plugins map[string][]string
}

func (pa *PluginAgent) Start(path string) {
	pa.tryLoad(path)
	ticker := time.Tick(1 * time.Minute)
	go func() {
		for range ticker {
			pa.tryLoad(path)
		}
	}()
}

func (pa *PluginAgent) Enabled(repo, plugin string) bool {
	pa.mut.Lock()
	defer pa.mut.Unlock()
	for _, p := range pa.plugins[repo] {
		if p == plugin {
			return true
		}
	}
	return false
}

// Hold the lock.
func (pa *PluginAgent) load(path string) error {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	np := map[string][]string{}
	if err := yaml.Unmarshal(b, &np); err != nil {
		return err
	}
	pa.plugins = np
	return nil
}

func (pa *PluginAgent) tryLoad(path string) {
	pa.mut.Lock()
	defer pa.mut.Unlock()
	if err := pa.load(path); err != nil {
		logrus.WithField("path", path).WithError(err).Error("Error loading plugin config.")
	}
}
