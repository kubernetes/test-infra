/*
Copyright The Kubernetes Authors.

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
	"regexp"
	"sort"

	prowconfig "sigs.k8s.io/prow/pkg/config"
)

// mainProwConfigPath is the location of the main Prow config, relative to
// the root of the test-infra repository. This tool must be invoked with the
// current working directory set to that root.
const mainProwConfigPath = "config/prow/config.yaml"

// listPeriodicJobs returns the sorted names of all periodic jobs defined
// underneath jobDir (a path relative to the test-infra repo root, e.g.
// "config/jobs/kubernetes/sig-node") whose name matches jobFilter and which
// test kubernetes/kubernetes at master.
func listPeriodicJobs(jobDir string, jobFilter *regexp.Regexp) ([]string, error) {
	c, err := prowconfig.Load(mainProwConfigPath, jobDir, nil, "")
	if err != nil {
		return nil, fmt.Errorf("loading Prow config for job dir %q: %w", jobDir, err)
	}

	var names []string
	for _, p := range c.AllPeriodics() {
		if !jobFilter.MatchString(p.Name) {
			continue
		}
		if !testsKubernetesAtMaster(p) {
			continue
		}
		names = append(names, p.Name)
	}
	sort.Strings(names)
	return names, nil
}

// testsKubernetesAtMaster returns whether p tests the kubernetes/kubernetes
// repository at its master branch, as opposed to not testing kubernetes at
// all (e.g. "auto-refreshing-official-cve-feed") or testing a previous
// release branch of it (e.g. "ci-kubernetes-gce-conformance-latest-1-33").
//
// A job is considered to test kubernetes/kubernetes if it either has an
// "extra_refs" entry with org "kubernetes", repo "kubernetes" and
// "auxiliary" not set to true (e.g. "ci-dra-integration"), or has the
// labels "prow.k8s.io/refs.org: kubernetes" and
// "prow.k8s.io/refs.repo: kubernetes" (e.g. "ci-kind-dra").
//
// If the matching "extra_refs" entry has a non-empty "base_ref" other than
// "master", the job is excluded since it tests a previous release.
func testsKubernetesAtMaster(p prowconfig.Periodic) bool {
	for _, ref := range p.ExtraRefs {
		if ref.Org != "kubernetes" || ref.Repo != "kubernetes" || ref.Auxiliary {
			continue
		}
		return ref.BaseRef == "" || ref.BaseRef == "master"
	}
	return p.Labels["prow.k8s.io/refs.org"] == "kubernetes" && p.Labels["prow.k8s.io/refs.repo"] == "kubernetes"
}
