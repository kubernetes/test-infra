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

// Package calculation calculates coverage through summarizing and filtering.
package calculation

import (
	"golang.org/x/tools/cover"
)

// ProduceCovList summarizes profiles and returns the result
func ProduceCovList(profiles []*cover.Profile) *CoverageList {
	covList := newCoverageList("summary")
	for _, prof := range profiles {
		covList.Group = append(covList.Group, summarizeBlocks(prof))
	}
	return covList
}

func summarizeBlocks(profile *cover.Profile) Coverage {
	cov := Coverage{Name: profile.FileName}
	for _, blk := range profile.Blocks {
		cov.NumAllStmts += blk.NumStmt
		if blk.Count > 0 {
			cov.NumCoveredStmts += blk.NumStmt
		}
	}
	return cov
}
