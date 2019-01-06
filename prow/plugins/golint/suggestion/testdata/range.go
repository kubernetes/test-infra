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

var m []string

func fOneValue() {
	// Should suggest `for range m {`
	for _ = range m {
	}
}

func fTwoValues() {
	// Should suggest `for range m {`
	for _, _ = range m {
	}
}

func fSecondValue() int {
	var y = 0
	// Should suggest `for y = range m {`
	for y, _ = range m {
	}
	return y
}

func fNoRangeError() int {
	var y = 0
	for y = range m {
	}
	return y
}
