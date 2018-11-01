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

// coverage stores test coverage summary data for one file
type coverage struct {
	Name          string
	nCoveredStmts int
	nAllStmts     int
}

// Ratio returns the percentage of statements that are covered
func (c *coverage) Ratio() float32 {
	if c.nAllStmts == 0 {
		return 1
	}
	return float32(c.nCoveredStmts) / float32(c.nAllStmts)
}
