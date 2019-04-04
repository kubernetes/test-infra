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
	"k8s.io/test-infra/prow/pjutil"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/gerrit/client"
)

// messageFilter builds a filter for jobs based on the messageBody matching the trigger regex of the jobs.
func messageFilter(lastUpdate time.Time, change client.ChangeInfo, presubmits []config.Presubmit) (pjutil.Filter, error) {
	var filters []pjutil.Filter
	currentRevision := change.Revisions[change.CurrentRevision].Number
	for _, message := range change.Messages {
		messageTime := message.Date.Time
		if message.RevisionNumber != currentRevision || messageTime.Before(lastUpdate) {
			continue
		}

		if !pjutil.TestAllRe.MatchString(message.Message) {
			for _, presubmit := range presubmits {
				if presubmit.TriggerMatches(message.Message) {
					logrus.Infof("Comment %s matches triggering regex, for %s.", message.Message, presubmit.Name)
					filters = append(filters, pjutil.CommandFilter(message.Message))
				}
			}
		} else {
			filters = append(filters, pjutil.TestAllFilter())
		}

	}
	return pjutil.AggregateFilter(filters), nil
}
