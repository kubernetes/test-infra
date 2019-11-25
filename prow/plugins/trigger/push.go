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
	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pjutil"
)

func listPushEventChanges(pe github.PushEvent) config.ChangedFilesProvider {
	return func() ([]string, error) {
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
		var changedFiles []string
		for file := range changed {
			changedFiles = append(changedFiles, file)
		}
		return changedFiles, nil
	}
}

func createRefs(pe github.PushEvent) prowapi.Refs {
	return prowapi.Refs{
		Org:      pe.Repo.Owner.Name,
		Repo:     pe.Repo.Name,
		BaseRef:  pe.Branch(),
		BaseSHA:  pe.After,
		BaseLink: pe.Compare,
	}
}

func handlePE(c Client, pe github.PushEvent) error {
	if pe.Deleted {
		// we should not trigger jobs for a branch deletion
		return nil
	}
	for _, j := range c.Config.Postsubmits[pe.Repo.FullName] {
		if shouldRun, err := j.ShouldRun(pe.Branch(), listPushEventChanges(pe)); err != nil {
			return err
		} else if !shouldRun {
			continue
		}
		refs := createRefs(pe)
		labels := make(map[string]string)
		for k, v := range j.Labels {
			labels[k] = v
		}
		labels[github.EventGUID] = pe.GUID
		pj := pjutil.NewProwJob(pjutil.PostsubmitSpec(j, refs), labels, j.Annotations)
		c.Logger.WithFields(pjutil.ProwJobFields(&pj)).Info("Creating a new prowjob.")
		if _, err := c.ProwJobClient.Create(&pj); err != nil {
			return err
		}
	}
	return nil
}
