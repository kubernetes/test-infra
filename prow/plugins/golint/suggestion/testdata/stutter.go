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

package testdata

// TestdataFunc comment
func TestdataFunc() error {
	// Should suggest `func Func() error {` for the line above
	return nil
}

// TestdataMaker comment
type TestdataMaker struct{} // Should suggest `type Maker struct {}`

// testdataMaker2 comment
type testdataMaker2 struct{} // No suggestion
