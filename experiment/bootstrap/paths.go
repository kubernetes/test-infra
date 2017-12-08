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

package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Paths contains all of the upload/file paths used in a run of bootstrap
type Paths struct {
	Artifacts     string
	BuildLog      string
	PRPath        string
	PRBuildLink   string
	PRLatest      string
	PRResultCache string
	ResultCache   string
	Started       string
	Finished      string
	Latest        string
}

// CIPaths returns a Paths for a CI Job
func CIPaths(base, job, build string) *Paths {
	return &Paths{
		Artifacts:   filepath.Join(base, job, build, "artifacts"),
		BuildLog:    filepath.Join(base, job, build, "build-log.txt"),
		Finished:    filepath.Join(base, job, build, "finished.json"),
		Latest:      filepath.Join(base, job, "latest-build.txt"),
		ResultCache: filepath.Join(base, job, "jobResultsCache.json"),
		Started:     filepath.Join(base, job, build, "started.json"),
	}
}

// PRPaths returns a Paths for a Pull Request
func PRPaths(base string, repos Repos, job, build string) (*Paths, error) {
	if len(repos) == 0 {
		return nil, fmt.Errorf("repos should not be empty")
	}
	repo := repos.Main()
	// TODO(bentheelder): can we make this more generic?
	var prefix string
	if repo.Name == "k8s.io/kubernetes" || repo.Name == "kubernetes/kubernetes" {
		prefix = ""
	} else if strings.HasPrefix(repo.Name, "k8s.io/") {
		prefix = repo.Name[len("k8s.io/"):]
	} else if strings.HasPrefix(repo.Name, "kubernetes/") {
		prefix = repo.Name[len("kubernetes/"):]
	} else if strings.HasPrefix(repo.Name, "github.com/") {
		prefix = strings.Replace(repo.Name[len("github.com/"):], "/", "_", -1)
	} else {
		prefix = strings.Replace(repo.Name, "/", "_", -1)
	}
	// Batch merges are those with more than one PR specified.
	prNums := repo.PullNumbers()
	var pull string
	switch len(prNums) {
	// TODO(bentheelder): jenkins/bootstrap.py would do equivalent to:
	// `pull = filepath.Join(prefix, repo.Pull)` in this case, though we
	// don't appear to ever have used this and probably shouldn't.
	// Revisit if we want to error here or do the previous screwy behavior
	case 0:
		return nil, fmt.Errorf("expected at least one PR number")
	case 1:
		pull = filepath.Join(prefix, prNums[0])
	default:
		pull = filepath.Join(prefix, "batch")
	}
	prPath := filepath.Join(base, "pull", pull, job, build)
	return &Paths{
		Artifacts:     filepath.Join(prPath, "artifacts"),
		BuildLog:      filepath.Join(prPath, "build-log.txt"),
		PRPath:        prPath,
		Finished:      filepath.Join(prPath, "finished.json"),
		Latest:        filepath.Join(base, "directory", job, "latest-build.txt"),
		PRBuildLink:   filepath.Join(base, "directory", job, build+".txt"),
		PRLatest:      filepath.Join(base, "pull", pull, job, "latest-build.txt"),
		PRResultCache: filepath.Join(base, "pull", pull, job, "jobResultsCache.json"),
		ResultCache:   filepath.Join(base, "directory", job, "jobResultsCache.json"),
		Started:       filepath.Join(prPath, "started.json"),
	}, nil
}

// GubernatorBuildURL returns a Gubernator link for this build.
func GubernatorBuildURL(paths *Paths) string {
	logPath := filepath.Dir(paths.BuildLog)
	if strings.HasPrefix(logPath, "gs:/") {
		return strings.Replace(logPath, "gs:/", GubernatorBaseBuildURL, 1)
	}
	return logPath
}
