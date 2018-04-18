/*
Copyright 2018 The Kubernetes Authors.

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

package tide

import (
	"testing"

	"k8s.io/apimachinery/pkg/util/sets"
)

func TestContextRegisterIgnoreContext(t *testing.T) {
	testCases := []struct {
		name               string
		required, optional []string
		contexts           []string
		results            []bool
	}{
		{
			name:     "only optional contexts registered",
			contexts: []string{"c1", "o1", "o2"},
			optional: []string{"o1", "o2"},
			results:  []bool{false, true, true},
		},
		{
			name:     "no contexts registered",
			contexts: []string{"t2"},
			results:  []bool{false},
		},
		{
			name:     "only required contexts registered",
			required: []string{"c1", "c2", "c3"},
			contexts: []string{"c1", "c2", "c3", "t1"},
			results:  []bool{false, false, false, true},
		},
		{
			name:     "optional and required contexts registered",
			optional: []string{"o1", "o2"},
			required: []string{"c1", "c2", "c3"},
			contexts: []string{"o1", "o2", "c1", "c2", "c3", "t1"},
			results:  []bool{true, true, false, false, false, true},
		},
	}

	for _, tc := range testCases {
		cr := NewContextRegister(tc.optional...)
		cr.RegisterRequiredContexts(tc.required...)
		for i, c := range tc.contexts {
			if cr.IgnoreContext(newExpectedContext(c)) != tc.results[i] {
				t.Errorf("%s - ignoreContext for %s should return %t", tc.name, c, tc.results[i])
			}
		}
	}
}

func contextsToSet(contexts []Context) sets.String {
	s := sets.NewString()
	for _, c := range contexts {
		s.Insert(string(c.Context))
	}
	return s
}

func TestContextRegisterMissingContexts(t *testing.T) {
	testCases := []struct {
		name                               string
		required, optional                 []string
		existingContexts, expectedContexts []string
	}{
		{
			name:             "no contexts registered",
			existingContexts: []string{"c1", "c2"},
		},
		{
			name:             "optional contexts registered / no missing contexts",
			optional:         []string{"o1", "o2", "o3"},
			existingContexts: []string{"c1", "c2"},
		},
		{
			name:             "required  contexts registered / missing contexts",
			required:         []string{"c1", "c2", "c3"},
			existingContexts: []string{"c1", "c2"},
			expectedContexts: []string{"c3"},
		},
		{
			name:             "required contexts registered / no missing contexts",
			required:         []string{"c1", "c2", "c3"},
			existingContexts: []string{"c1", "c2", "c3"},
		},
		{
			name:             "optional and required contexts registered / missing contexts",
			optional:         []string{"o1", "o2", "o3"},
			required:         []string{"c1", "c2", "c3"},
			existingContexts: []string{"c1", "c2"},
			expectedContexts: []string{"c3"},
		},
		{
			name:             "optional and required contexts registered / no missing contexts",
			optional:         []string{"o1", "o2", "o3"},
			required:         []string{"c1", "c2"},
			existingContexts: []string{"c1", "c2", "c4"},
		},
	}

	for _, tc := range testCases {
		cr := NewContextRegister(tc.optional...)
		cr.RegisterRequiredContexts(tc.required...)
		var contexts []Context
		for _, c := range tc.existingContexts {
			contexts = append(contexts, newExpectedContext(c))
		}
		missingContexts := cr.MissingRequiredContexts(contexts)
		m := contextsToSet(missingContexts)
		if !m.Equal(sets.NewString(tc.expectedContexts...)) {
			t.Errorf("%s - expected %v got %v", tc.name, tc.expectedContexts, missingContexts)
		}
	}
}
