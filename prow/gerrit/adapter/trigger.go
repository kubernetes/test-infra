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
	"fmt"
	"k8s.io/test-infra/prow/pjutil"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/gerrit/client"
)

const layout = "2006-01-02 15:04:05"

func filterPresubmits(filter pjutil.Filter, change client.ChangeInfo, presubmits []config.Presubmit) ([]config.Presubmit, []config.Presubmit, error) {
	var toTrigger []config.Presubmit
	var toSkip []config.Presubmit

	for _, presubmit := range presubmits {
		matches, forced, defaults := filter(presubmit)
		fmt.Printf("job name: %v, matches: %v\n", presubmit.Name, matches)
		if !matches {
			continue
		}

		shouldRun, err := presubmit.ShouldRun(change.Branch, listChangedFiles(change), forced, defaults)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to determine if presubmit %q should run: %v", presubmit.Name, err)
		}

		if shouldRun {
			toTrigger = append(toTrigger, presubmit)
		} else {
			toSkip = append(toSkip, presubmit)
		}
	}
	logrus.WithFields(logrus.Fields{"gerrit change": change.Number, "to-trigger": toTrigger, "to-skip": toSkip}).Debugf("Filtered %d jobs, found %d to trigger and %d to skip.", len(presubmits), len(toTrigger), len(toSkip))
	return toTrigger, toSkip, nil
}

// messageFilter builds a filter for jobs based on the messageBody matching the trigger regex of the jobs.
func messageFilter(lastUpdate time.Time, change client.ChangeInfo, presubmits []config.Presubmit) (pjutil.Filter, error) {
	var filters []pjutil.Filter
	currentRevision := change.Revisions[change.CurrentRevision].Number
	for _, message := range change.Messages {
		messageTime, err := time.Parse(layout, message.Date)
		if err != nil {
			logrus.WithError(err).Errorf("Parse time %v failed", message.Date)
			continue
		}
		if message.RevisionNumber != currentRevision || messageTime.Before(lastUpdate) {
			continue

		}
		// Skip comments not germane to this plugin
		if !pjutil.TestAllRe.MatchString(message.Message) {
			matched := false
			for _, presubmit := range presubmits {
				matched = matched || presubmit.TriggerMatches(message.Message)
				if matched {
					filters = append(filters, pjutil.CommandFilter(message.Message))
				}
			}
			if !matched {
				logrus.Infof("Comment %s doesn't match any triggering regex, skipping.", message.Message)
				continue
			}

		} else {
			filters = append(filters, pjutil.TestAllFilter())
		}

	}
	return pjutil.AggregateFilter(filters), nil
}

// oldRevisionFilter builds a filter for jobs that have already been processed for a revision.
func oldRevisionFilter(lastUpdate time.Time, rev client.RevisionInfo) pjutil.Filter {
	created, err := time.Parse(layout, rev.Created)
	if err != nil {
		logrus.WithError(err).Errorf("Parse time %v failed", rev.Created)
		return func(p config.Presubmit) (bool, bool, bool) {
			return false, false, false
		}
	}

	return func(p config.Presubmit) (bool, bool, bool) {
		return created.After(lastUpdate), false, false
	}
}
