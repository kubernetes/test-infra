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

import "testing"

// echo -n 'BODY' | openssl dgst -sha1 -hmac KEY
func TestValidatePayload(t *testing.T) {
	var testcases = []struct {
		payload string
		sig     string
		key     string
		valid   bool
	}{
		{
			"{}",
			"sha1=db5c76f4264d0ad96cf21baec394964b4b8ce580",
			"abc",
			true,
		},
		{
			"{}",
			"db5c76f4264d0ad96cf21baec394964b4b8ce580",
			"abc",
			false,
		},
		{
			"{}",
			"",
			"abc",
			false,
		},
		{
			"",
			"sha1=cc47e3c0aa0c2984454476d061108c0b110177ae",
			"abc",
			true,
		},
		{
			"",
			"sha1=fbdb1d1b18aa6c08324b7d64b71fb76370690e1d",
			"",
			true,
		},
		{
			"{}",
			"",
			"abc",
			false,
		},
	}
	for _, tc := range testcases {
		if ValidatePayload([]byte(tc.payload), tc.sig, []byte(tc.key)) != tc.valid {
			t.Errorf("Wrong validation for %+v", tc)
		}
	}
}
