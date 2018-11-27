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

package trigger

import (
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pjutil"
)

func listPushEventChanges(pe github.PushEvent) []string {
	changed := make(map[string]bool)
	for _, commit := range pe.Commits {
		for _, added := range commit.Added {
			changed[added] = true
		}
		for _, removed := range commit.Removed {
			changed[removed] = true
		}
		for _, modified := range commit.Modified {
			changed[modified] = true
		}
	}
	changedFiles := []string{}
	for file := range changed {
		changedFiles = append(changedFiles, file)
	}
	return changedFiles
}

func handlePE(c client, pe github.PushEvent) error {
	for _, j := range c.Config.Postsubmits[pe.Repo.FullName] {
		if !j.RunsAgainstBranch(pe.Branch()) {
			continue
		}
		changedFiles := listPushEventChanges(pe)
		if !j.RunsAgainstChanges(changedFiles) {
			continue
		}
		kr := kube.Refs{
			Org:     pe.Repo.Owner.Name,
			Repo:    pe.Repo.Name,
			BaseRef: pe.Branch(),
			BaseSHA: pe.After,
		}
		labels := make(map[string]string)
		for k, v := range j.Labels {
			labels[k] = v
		}
		labels[github.EventGUID] = pe.GUID
		pj := pjutil.NewProwJob(pjutil.PostsubmitSpec(j, kr), labels)
		c.Logger.WithFields(pjutil.ProwJobFields(&pj)).Info("Creating a new prowjob.")
		if _, err := c.KubeClient.CreateProwJob(pj); err != nil {
			return err
		}
	}
	return nil
}
