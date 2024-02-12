/*
Copyright 2024 The Kubernetes Authors.

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

package resultstore

import "testing"

func TestTokenGenerator(t *testing.T) {

	for _, tc := range []struct {
		desc string
		seed string
		id   string
		want string
	}{
		{
			desc: "normal",
			seed: "Test Seed",
			id:   "2cf762a1-e2ff-4c09-a45c-96ace79c0080",
			want: "a6ca96c8-5411-4767-8cde-2e886cea8fea",
		},
		{
			desc: "empty id",
			seed: "Test Seed",
			id:   "",
			want: "dd84c04e-5803-447b-b2dd-cc85c6d01281",
		},
		{
			desc: "empty seed",
			seed: "",
			id:   "2cf762a1-e2ff-4c09-a45c-96ace79c0080",
			want: "35f1d874-5264-4b2d-afd5-cbbcfd43be37",
		},
		{
			desc: "non uuid ok",
			seed: "Test Seed",
			id:   "non-uuid-value",
			want: "c89d28cc-0f73-43c1-ba3a-695570eb3546",
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			tg := tokenGenerator{}
			tg.Reseed(tc.seed)
			if got, want := tg.From(tc.id), tc.want; got != want {
				t.Errorf("From() got %q, want %q", got, want)
			}
			// Ensure repeatable values.
			if got, want := tg.From(tc.id), tc.want; got != want {
				t.Errorf("Second From() got %q, want %q", got, want)
			}
		})
	}
}
