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

// version holds variables that identify a Prow binary's name and version
package version

import "testing"

func TestVersionTimestamp(t *testing.T) {
	tests := []struct {
		name    string
		version string
		wantV   int64
		wantErr bool
	}{
		{
			"base case",
			"v20200102-a1b2c3",
			1577923200,
			false,
		},
		{
			"invalid version",
			"v20200102a-a1b2c3",
			0,
			true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			Version = tc.version
			got, gotErr := VersionTimestamp()
			if got != tc.wantV {
				t.Fatalf("version mismatch, want: %d, got: %d", tc.wantV, got)
			}
			if ((gotErr != nil) && tc.wantErr) != ((gotErr != nil) || tc.wantErr) {
				t.Fatalf("error mismatch, want: %v, got: %v", tc.wantErr, gotErr)
			}
		})
	}
}
