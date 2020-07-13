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

package github

import (
	"testing"
)

var tokens = `
'*':
  - value: abc
    created_at: 2020-10-02T15:00:00Z
  - value: key
    created_at: 2018-10-02T15:00:00Z
'org1':
  - value: abc1
    created_at: 2020-10-02T15:00:00Z
  - value: key1
    created_at: 2018-10-02T15:00:00Z
'org2/repo':
  - value: abc2
    created_at: 2020-10-02T15:00:00Z
  - value: key2
    created_at: 2018-10-02T15:00:00Z
`

var defaultTokenGenerator = func() []byte {
	return []byte(tokens)
}

// echo -n 'BODY' | openssl dgst -sha1 -hmac KEY
func TestValidatePayload(t *testing.T) {
	var testcases = []struct {
		name           string
		payload        string
		sig            string
		tokenGenerator func() []byte
		valid          bool
	}{
		{
			"empty payload with a correct signature can pass the check",
			"{}",
			"sha1=db5c76f4264d0ad96cf21baec394964b4b8ce580",
			defaultTokenGenerator,
			true,
		},
		{
			"empty payload with a wrong formatted signature cannot pass the check",
			"{}",
			"db5c76f4264d0ad96cf21baec394964b4b8ce580",
			defaultTokenGenerator,
			false,
		},
		{
			"empty signature is not valid",
			"{}",
			"",
			defaultTokenGenerator,
			false,
		},
		{
			"org-level webhook event with a correct signature can pass the check",
			`{"organization": {"login": "org1"}}`,
			"sha1=cf2d7e20aa4863abe204a61a8adf53ddaef0b33b",
			defaultTokenGenerator,
			true,
		},
		{
			"repo-level webhook event with a correct signature can pass the check",
			`{"repository": {"full_name": "org2/repo"}}`,
			"sha1=0b5ea8bf5683e4bf89cf900271e1c8a021b4b0b3",
			defaultTokenGenerator,
			true,
		},
		{
			"payload with both repository and organization is considered as a repo-level webhook event",
			`{"repository": {"full_name": "org2/repo"}, "organization": {"login": "org2"}}`,
			"sha1=db5ba00c9ed0153322d33decb7ad579401e917f6",
			defaultTokenGenerator,
			true,
		},
	}
	for _, tc := range testcases {
		res := ValidatePayload([]byte(tc.payload), tc.sig, tc.tokenGenerator)
		if res != tc.valid {
			t.Errorf("Wrong validation for the test %q: expected %t but got %t", tc.name, tc.valid, res)
		}
	}
}
