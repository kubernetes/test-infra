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
	"k8s.io/test-infra/prow/plank"
)

func handlePE(c client, pe github.PushEvent) error {
	for _, j := range c.Config.Postsubmits[pe.Repo.FullName] {
		if !j.RunsAgainstBranch(pe.Branch()) {
			continue
		}
		kr := kube.Refs{
			Org:     pe.Repo.Owner.Name,
			Repo:    pe.Repo.Name,
			BaseRef: pe.Branch(),
			BaseSHA: pe.After,
		}
		if _, err := c.KubeClient.CreateProwJob(plank.NewProwJob(plank.PostsubmitSpec(j, kr))); err != nil {
			return err
		}
	}
	return nil
}
