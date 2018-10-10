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

package artifacts

import (
	"testing"

	"k8s.io/test-infra/coverage/test"
)

// generates coverage profile by running go test on target package
func TestComposeCmdArgs(t *testing.T) {
	t.Run("target=.", func(t *testing.T) {
		expected := []string{"test", ".", "-covermode=count", "-coverprofile", "./path/to/profile"}
		actual, _ := composeCmdArgs(".", "./path/to/profile")
		test.AssertDeepEqual(t, expected, actual)
	})
	t.Run("target=./...", func(t *testing.T) {
		expected := []string{"test", "./...", "-covermode=count", "-coverprofile", "./path/to/profile"}
		actual, _ := composeCmdArgs("./...", "./path/to/profile")
		test.AssertDeepEqual(t, expected, actual)
	})
	t.Run("target=./pkg ./cmd/...", func(t *testing.T) {
		expected := []string{"test", "./pkg", "./cmd/...", "-covermode=count", "-coverprofile", "./path/to/profile"}
		actual, _ := composeCmdArgs("./pkg ./cmd/...", "./path/to/profile")
		test.AssertDeepEqual(t, expected, actual)
	})
	t.Run("target=/absolute", func(t *testing.T) {
		expected := "target path can not be absolute path: Path='/absolute'"
		_, err := composeCmdArgs("/absolute", "./path/to/profile")
		if err == nil {
			t.Errorf("failed to catch error")
		}
		test.AssertEqual(t, expected, err.Error())
	})
}
