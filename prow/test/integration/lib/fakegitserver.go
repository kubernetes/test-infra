/*
Copyright 2022 The Kubernetes Authors.

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

package lib

type FGSRepoSetup struct {
	// Name of the Git repo. It will get a ".git" appended to it and be
	// initialized underneath o.gitReposParentDir.
	Name string `json:"name"`
	// Script to execute. This script runs inside the repo to perform any
	// additional repo setup tasks. This script is executed by /bin/sh.
	Script string `json:"script"`
	// Whether to create the repo at the path (o.gitReposParentDir + name +
	// ".git") even if a file (directory) exists there already. This basically
	// does a 'rm -rf' of the folder first.
	Overwrite bool `json:"overwrite"`
}
