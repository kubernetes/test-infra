/*
Copyright 2021 The Kubernetes Authors.

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

package fuzztest

func Fuzz(data []byte) int {
	testData := []string{}
	if len(data) < 2 {
		return 0
	}
	if string(data[0]) == "h" && string(data[1]) == "i" {
		x := testData[10000]
		return int(len(x))
	}
	return 0
}
