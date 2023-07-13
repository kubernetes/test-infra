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

	"github.com/andygrunwald/go-gerrit"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/gerrit/client"
	"k8s.io/test-infra/prow/pjutil"
)

// presubmitContexts returns the set of failing and all job names contained in the reports.
func presubmitContexts(failed sets.Set[string], presubmits []config.Presubmit, logger logrus.FieldLogger) (sets.Set[string], sets.Set[string]) {
	allContexts := sets.Set[string]{}
	for _, presubmit := range presubmits {
		allContexts.Insert(presubmit.Name) // TODO(fejta): context, not name
	}
	failedContexts := allContexts.Intersection(failed)
	return failedContexts, allContexts
}

// currentMessages returns messages on the current revision after the specified time.
func currentMessages(change gerrit.ChangeInfo, since time.Time) []gerrit.ChangeMessageInfo {
	var messages []gerrit.ChangeMessageInfo
	want := change.Revisions[change.CurrentRevision].Number
	for _, have := range change.Messages {
		if have.RevisionNumber != want {
			continue
		}
		if !have.Date.Time.After(since) {
			continue
		}
		messages = append(messages, have)
	}
	return messages
}

// messageFilter returns filter that matches all /test all, /test foo, /retest comments since lastUpdate.
//
// The behavior of each message matches the behavior of pjutil.PresubmitFilter.
func messageFilter(messages []gerrit.ChangeMessageInfo, failingContexts, allContexts sets.Set[string], triggerTimes map[string]time.Time, logger logrus.FieldLogger) pjutil.Filter {
	var filters []pjutil.Filter
	contextGetter := func() (sets.Set[string], sets.Set[string], error) {
		return failingContexts, allContexts, nil
	}
	for _, message := range messages {
		// Use the PresubmitFilter before possibly adding the /test-all filter to ensure explicitly requested presubmits are forced to run.
		filter, err := pjutil.PresubmitFilter(false, contextGetter, message.Message, logger)
		if err != nil {
			logger.WithError(err).WithField("message", message).Warn("failed to create presubmit filter")
			continue
		}
		filters = append(filters, &timeAnnotationFilter{
			Filter:       filter,
			eventTime:    message.Date.Time,
			triggerTimes: triggerTimes,
		})
		// If the Gerrit Change changed from draft to active state, trigger all
		// presubmit Prow jobs.
		if strings.HasSuffix(message.Message, client.ReadyForReviewMessageFixed) || strings.HasSuffix(message.Message, client.ReadyForReviewMessageCustomizable) {
			filters = append(filters, &timeAnnotationFilter{
				Filter:       pjutil.NewTestAllFilter(),
				eventTime:    message.Date.Time,
				triggerTimes: triggerTimes,
			})
			continue
		}
	}

	return pjutil.NewAggregateFilter(filters)
}

// timeAnnotationFilter is a wrapper around a pjutil.Filter that records the eventTime in
// the triggerTimes map when the Filter returns a true 'shouldRun' value.
type timeAnnotationFilter struct {
	pjutil.Filter                      // Delegate filter. Only override/wrap ShouldRun() for time annotation, use Name() directly.
	eventTime     time.Time            // The time of the event. If the filter matches, use this time for annotation.
	triggerTimes  map[string]time.Time // This map is referenced by all timeAnnotationFilters for a single processing iteration.
}

func (taf *timeAnnotationFilter) ShouldRun(p config.Presubmit) (shouldRun bool, forcedToRun bool, defaultBehavior bool) {
	shouldRun, forced, def := taf.Filter.ShouldRun(p)
	if shouldRun {
		taf.triggerTimes[p.Name] = taf.eventTime
	}
	return shouldRun, forced, def
}
