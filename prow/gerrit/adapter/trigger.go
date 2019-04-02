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

package adapter

import (
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/gerrit/client"
	"k8s.io/test-infra/prow/gerrit/reporter"
	"k8s.io/test-infra/prow/pjutil"
)

// messageFilter builds a filter for jobs based on the messageBody matching the trigger regex of the jobs.
func messageFilter(lastUpdate time.Time, change client.ChangeInfo, presubmits []config.Presubmit, latestReport *reporter.JobReport) (pjutil.Filter, error) {
	var filters []pjutil.Filter
	currentRevision := change.Revisions[change.CurrentRevision].Number
	for _, message := range change.Messages {
		messageTime := message.Date.Time
		if message.RevisionNumber != currentRevision || !messageTime.After(lastUpdate) {
			continue
		}

		if pjutil.TestAllRe.MatchString(message.Message) {
			filters = append(filters, pjutil.TestAllFilter())
		} else if pjutil.RetestRe.MatchString(message.Message) {
			if latestReport == nil {
				continue
			}
			allContexts := sets.String{}
			failedContexts := sets.String{}

			jobs := map[string]*reporter.Job{}
			for _, job := range latestReport.Jobs {
				jobs[job.Name] = job
			}
			for _, presubmit := range presubmits {
				allContexts.Insert(presubmit.Name)
				j, ok := jobs[presubmit.Name]
				if ok && strings.ToLower(j.State) == string(v1.FailureState) {
					failedContexts.Insert(presubmit.Name)
				}
			}
			filters = append(filters, pjutil.RetestFilter(failedContexts, allContexts))
		} else {
			for _, presubmit := range presubmits {
				if presubmit.TriggerMatches(message.Message) {
					logrus.Infof("Change %d: Comment %s matches triggering regex, for %s.", change.Number, message.Message, presubmit.Name)
					filters = append(filters, pjutil.CommandFilter(message.Message))
				}
			}

		}

	}
	return pjutil.AggregateFilter(filters), nil
}
