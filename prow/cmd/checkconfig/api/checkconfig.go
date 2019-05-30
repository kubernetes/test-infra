/*
Copyright 2019 The Kubernetes Authors.

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

package checkconfig

import (
	"fmt"

	"k8s.io/apimachinery/pkg/util/validation"
	v1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
)

func ValidatePresubmitJob(repo string, job config.Presubmit) error {
	// Prow labels k8s resources with job names. Labels are capped at 63 chars.
	if job.Agent == string(v1.KubernetesAgent) && len(job.Name) > validation.LabelValueMaxLength {
		return fmt.Errorf("name of Presubmit job %q (for repo %q) too long (should be at most 63 characters)", job.Name, repo)
	}
	return nil
}
