/*
Copyright 2020 The Kubernetes Authors.

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

package yaml

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

type simple struct {
	Field string `json:"zz_field"`
}

func TestRoundTripping(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		in       []byte
		target   interface{}
		expected interface{}
	}{
		{
			name: "simple",
			in: []byte(`zz_field: val
`),
			target:   &simple{},
			expected: &simple{Field: "val"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if err := Unmarshal(tc.in, tc.target); err != nil {
				t.Fatalf("unmarshaling errored: %v", err)
			}
			if diff := cmp.Diff(tc.target, tc.expected); diff != "" {
				t.Errorf("target differs from expected: %s", diff)
			}

			reserialized, err := Marshal(tc.target)
			if err != nil {
				t.Fatalf("unmarshaling errored: %v", err)
			}
			if diff := cmp.Diff(string(tc.in), string(reserialized)); diff != "" {
				t.Errorf("got diff after reserializing: %s", diff)
			}
		})
	}
}

func TestUnmarshal_anchors(t *testing.T) {
	raw := []byte(`
n: &anchorName AnchorValue

name: *anchorName
`)
	target := struct {
		Name string
	}{}
	if err := Unmarshal(raw, &target); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if target.Name != "AnchorValue" {
		t.Errorf("expected .Names value to be 'AnchorValue', was %s", target.Name)
	}
}
