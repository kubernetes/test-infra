/*
Copyright 2021 The Kubernetes Authors.

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

package secretutil

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestReloadingCensorer(t *testing.T) {
	text := func() []byte {
		return []byte("secret SECRET c2VjcmV0 sEcReT")
	}
	var testCases = []struct {
		name     string
		mutation func(c *ReloadingCensorer)
		expected []byte
	}{
		{
			name:     "no registered secrets",
			mutation: func(c *ReloadingCensorer) {},
			expected: text(),
		},
		{
			name: "registered strings",
			mutation: func(c *ReloadingCensorer) {
				c.Refresh("secret")
			},
			expected: []byte("****** SECRET ******** sEcReT"),
		},
		{
			name: "registered strings with padding",
			mutation: func(c *ReloadingCensorer) {
				c.Refresh("		secret      ")
			},
			expected: []byte("****** SECRET ******** sEcReT"),
		},
		{
			name: "registered strings only padding",
			mutation: func(c *ReloadingCensorer) {
				c.Refresh("		      ")
			},
			expected: text(),
		},
		{
			name: "registered multiple strings",
			mutation: func(c *ReloadingCensorer) {
				c.Refresh("secret", "SECRET", "sEcReT")
			},
			expected: []byte("****** ****** ******** ******"),
		},
		{
			name: "registered bytes",
			mutation: func(c *ReloadingCensorer) {
				c.RefreshBytes([]byte("secret"))
			},
			expected: []byte("****** SECRET ******** sEcReT"),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			censorer := NewCensorer()
			testCase.mutation(censorer)
			input := text()
			censorer.Censor(&input)
			if len(input) != len(text()) {
				t.Errorf("%s: length of input changed from %d to %d", testCase.name, len(text()), len(input))
			}
			if diff := cmp.Diff(testCase.expected, input); diff != "" {
				t.Errorf("%s: got incorrect text after censor: %v", testCase.name, diff)
			}
		})
	}
}
