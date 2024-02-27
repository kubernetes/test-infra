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

package testutil

import "fmt"

// CmpErrors shallow compares two errors and returns whether
// the comparison took place along with any differences.
func CmpErrors(wantErr, err error) (bool, string) {
	switch {
	case wantErr != nil && err != nil:
		if expected, actual := wantErr.Error(), err.Error(); expected != actual {
			return true, fmt.Sprintf("expected %q but got %q", expected, actual)
		}
		return true, ""
	case wantErr != nil && err == nil:
		return true, fmt.Sprintf("expected %q but got nil", wantErr.Error())
	case wantErr == nil && err != nil:
		return true, fmt.Sprintf("expected nil but got %q", err.Error())
	}
	return false, ""
}
