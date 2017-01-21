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
	"reflect"
	"runtime"
	"testing"

	github_util "k8s.io/contrib/mungegithub/github"
	"k8s.io/kubernetes/pkg/util/sets"
)

var (
	aliasYaml = `
aliases:
  team/t1:
    - u1
    - u2
  team/t2:
    - u1
    - u3`
)

type aliasTest struct{}

func (a *aliasTest) read() ([]byte, error) {
	return []byte(aliasYaml), nil
}

func TestExpandAliases(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())

	tests := []struct {
		name      string
		aliasFile string
		owners    sets.String
		expected  sets.String
	}{
		{
			name:     "No expansions.",
			owners:   sets.NewString("abc", "def"),
			expected: sets.NewString("abc", "def"),
		},
		{
			name:     "No aliases to be expanded",
			owners:   sets.NewString("abc", "team/t1"),
			expected: sets.NewString("abc", "u1", "u2"),
		},
		{
			name:     "Duplicates inside and outside alias.",
			owners:   sets.NewString("u1", "team/t1"),
			expected: sets.NewString("u1", "u2"),
		},
		{
			name:     "Duplicates in multiple aliases.",
			owners:   sets.NewString("u1", "team/t1", "team/t2"),
			expected: sets.NewString("u1", "u2", "u3"),
		},
	}

	for _, test := range tests {
		a := Aliases{
			aliasReader: &aliasTest{},
			IsEnabled:   true,
		}

		if err := a.Initialize(&github_util.Config{}); err != nil {
			t.Fatalf("%v", err)
		}

		if err := a.EachLoop(); err != nil {
			t.Fatalf("%v", err)
		}

		expanded := a.Expand(test.owners)
		if !reflect.DeepEqual(expanded, test.expected) {
			t.Errorf("%s: expected: %#v, got: %#v", test.name, test.expected, expanded)
		}
	}
}
