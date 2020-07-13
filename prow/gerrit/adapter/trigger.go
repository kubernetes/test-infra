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
	"time"

	"github.com/andygrunwald/go-gerrit"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/pjutil"
)

// presubmitContexts returns the set of failing and all job names contained in the reports.
func presubmitContexts(failed sets.String, presubmits []config.Presubmit, logger logrus.FieldLogger) (sets.String, sets.String) {
	allContexts := sets.String{}
	for _, presubmit := range presubmits {
		allContexts.Insert(presubmit.Name) // TODO(fejta): context, not name
	}
	failedContexts := allContexts.Intersection(failed)
	return failedContexts, allContexts
}

// currentMessages returns messages on the current revision after the specified time.
func currentMessages(change gerrit.ChangeInfo, since time.Time) []string {
	var messages []string
	want := change.Revisions[change.CurrentRevision].Number
	for _, have := range change.Messages {
		if have.RevisionNumber != want {
			continue
		}
		if !have.Date.Time.After(since) {
			continue
		}
		messages = append(messages, have.Message)
	}
	return messages
}

// messageFilter returns filter that matches all /test all, /test foo, /retest comments since lastUpdate.
//
// The behavior of each message matches the behavior of pjutil.PresubmitFilter.
func messageFilter(messages []string, failingContexts, allContexts sets.String, logger logrus.FieldLogger) pjutil.Filter {
	var filters []pjutil.Filter
	contextGetter := func() (sets.String, sets.String, error) {
		return failingContexts, allContexts, nil
	}
	for _, message := range messages {
		filter, err := pjutil.PresubmitFilter(false, contextGetter, message, logger)
		if err != nil {
			logger.WithError(err).WithField("message", message).Warn("failed to create presubmit filter")
			continue
		}
		filters = append(filters, filter)
	}
	return pjutil.AggregateFilter(filters)
}
