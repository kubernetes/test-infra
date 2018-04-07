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

package branchprotection

import (
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/util/diff"
)

func TestMergePolicy(test *testing.T) {
	t := true
	f := false
	basic := Policy{
		Protect: &t,
	}
	ebasic := Policy{
		Protect: &t,
	}
	cases := []struct {
		name     string
		parent   *Policy
		child    *Policy
		expected *Policy
	}{
		{
			name:     "nil child",
			parent:   &basic,
			expected: &ebasic,
		},
		{
			name: "merge parent and child",
			parent: &Policy{
				Protect: &t,
			},
			child: &Policy{
				Admins: &f,
			},
			expected: &Policy{
				Protect: &t,
				Admins:  &f,
			},
		},
		{
			name: "child overrides parent",
			parent: &Policy{
				Protect: &t,
			},
			child: &Policy{
				Protect: &f,
			},
			expected: &Policy{
				Protect: &f,
			},
		},
		{
			name: "append strings",
			parent: &Policy{
				ContextPolicy: &ContextPolicy{
					Contexts: []string{"hello", "world"},
				},
			},
			child: &Policy{
				ContextPolicy: &ContextPolicy{
					Contexts: []string{"world", "of", "thrones"},
				},
			},
			expected: &Policy{
				ContextPolicy: &ContextPolicy{
					Contexts: []string{"hello", "of", "thrones", "world"},
				},
			},
		},
		{
			name: "merge struct",
			parent: &Policy{
				ContextPolicy: &ContextPolicy{
					Contexts: []string{"hi"},
				},
			},
			child: &Policy{
				ContextPolicy: &ContextPolicy{
					Strict: &t,
				},
			},
			expected: &Policy{
				ContextPolicy: &ContextPolicy{
					Contexts: []string{"hi"},
					Strict:   &t,
				},
			},
		},
		{
			name: "nil child struct",
			parent: &Policy{
				ContextPolicy: &ContextPolicy{
					Strict: &f,
				},
			},
			child: &Policy{
				Protect: &t,
			},
			expected: &Policy{
				ContextPolicy: &ContextPolicy{
					Strict: &f,
				},
				Protect: &t,
			},
		},
		{
			name: "nil parent struct",
			child: &Policy{
				ContextPolicy: &ContextPolicy{
					Strict: &f,
				},
			},
			parent: &Policy{
				Protect: &t,
			},
			expected: &Policy{
				ContextPolicy: &ContextPolicy{
					Strict: &f,
				},
				Protect: &t,
			},
		},
	}

	for _, tc := range cases {
		test.Run(tc.name, func(test *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					test.Errorf("unexpected panic: %s", r)
				}
			}()
			actual := MergePolicy(tc.parent, tc.child)
			if !reflect.DeepEqual(actual, tc.expected) {
				test.Errorf("bad merged policy:\n%s", diff.ObjectReflectDiff(tc.expected, actual))
			}
		})
	}
}
