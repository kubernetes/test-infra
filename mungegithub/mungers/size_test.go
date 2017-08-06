/*
Copyright 2017 The Kubernetes Authors.

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

package mungers

import (
	"testing"
)

func TestCalculateSize(t *testing.T) {
	tests := []struct {
		adds int
		dels int
		size string
	}{
		{
			adds: 0,
			dels: 0,
			size: sizeXS,
		},
		{
			adds: 5,
			dels: 3,
			size: sizeXS,
		},
		{
			adds: 8,
			dels: 2,
			size: sizeS,
		},
		{
			adds: 20,
			dels: 40,
			size: sizeM,
		},
		{
			adds: 100,
			dels: 300,
			size: sizeL,
		},
		{
			adds: 600,
			dels: 100,
			size: sizeXL,
		},
		{
			adds: 1000,
			dels: 10,
			size: sizeXXL,
		},
	}

	for _, test := range tests {
		size := calculateSize(test.adds, test.dels)
		if size != test.size {
			t.Errorf("Expected size: %s, got: %s", test.size, size)
		}
	}
}
