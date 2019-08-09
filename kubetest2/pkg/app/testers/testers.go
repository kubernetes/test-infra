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

// Package testers is a registry of kubetest2 testers
package testers

import (
	"sync"

	"github.com/pkg/errors"

	"k8s.io/test-infra/kubetest2/pkg/types"
)

type testerInfo struct {
	tester types.NewTester
	usage  string
}

var (
	mu sync.Mutex
	// protected by mu
	registry = make(map[string]testerInfo)
)

// Get looks up a tester implementation by name, returning the tester if
// the name exists in the registry, it also additionally returns the existence
// explicitly
func Get(name string) (tester types.NewTester, usage string, exists bool) {
	mu.Lock()
	defer mu.Unlock()
	t, o := registry[name]
	return t.tester, t.usage, o
}

// Register registers a tester implementation by name
func Register(name, usage string, tester types.NewTester) error {
	mu.Lock()
	defer mu.Unlock()
	if _, exists := registry[name]; exists {
		return errors.Errorf("tester by name %#v already exists", name)
	}
	registry[name] = testerInfo{
		tester: tester,
		usage:  usage,
	}
	return nil
}

// Names returns a slice of all registered tester names.
func Names() []string {
	result := make([]string, 0, len(registry))
	for k := range registry {
		result = append(result, k)
	}
	return result
}
