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

package calculation

import (
	"testing"
)

func TestRatio(t *testing.T) {
	t.Run("regular", func(t *testing.T) {
		c := &Coverage{Name: "fake-coverage", NumCoveredStmts: 105, NumAllStmts: 210}
		actualRatio := c.Ratio()
		if actualRatio != float32(.5) {
			t.Fatalf("incorrect coverage ratio: expected 0.5, got %f", actualRatio)
		}
	})

	t.Run("no actual statements", func(t *testing.T) {
		c := &Coverage{Name: "fake-coverage", NumCoveredStmts: 0, NumAllStmts: 0}
		if c.Ratio() != float32(1) {
			t.Fatalf("incorrect coverage ratio: expected 1, got %f", c.Ratio())
		}
	})
}
