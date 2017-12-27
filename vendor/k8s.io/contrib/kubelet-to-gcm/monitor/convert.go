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

package monitor

// Float64Ptr gives a pointer to a variable with the same value as f.
func Float64Ptr(f float64) *float64 {
	return &f
}

// Int64Ptr gives a pointer to a variable with the same value as i.
func Int64Ptr(i int64) *int64 {
	return &i
}
