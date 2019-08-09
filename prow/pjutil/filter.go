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

package pjutil

import (
	"regexp"

	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/config"
)

var TestAllRe = regexp.MustCompile(`(?m)^/test all,?($|\s.*)`)

// RetestRe provides the regex for `/retest`
var RetestRe = regexp.MustCompile(`(?m)^/retest\s*$`)

var OkToTestRe = regexp.MustCompile(`(?m)^/ok-to-test\s*$`)

// Filter digests a presubmit config to determine if:
//  - we the presubmit matched the filter
//  - we know that the presubmit is forced to run
//  - what the default behavior should be if the presubmit
//    runs conditionally and does not match trigger conditions
type Filter func(p config.Presubmit) (shouldRun bool, forcedToRun bool, defaultBehavior bool)

// CommandFilter builds a filter for `/test foo`
func CommandFilter(body string) Filter {
	return func(p config.Presubmit) (bool, bool, bool) {
		return p.TriggerMatches(body), p.TriggerMatches(body), true
	}
}

// TestAllFilter builds a filter for the automatic behavior of `/test all`.
// Jobs that explicitly match `/test all` in their trigger regex will be
// handled by a commandFilter for the comment in question.
func TestAllFilter() Filter {
	return func(p config.Presubmit) (bool, bool, bool) {
		return !p.NeedsExplicitTrigger(), false, false
	}
}

// AggregateFilter builds a filter that evaluates the child filters in order
// and returns the first match
func AggregateFilter(filters []Filter) Filter {
	return func(presubmit config.Presubmit) (bool, bool, bool) {
		for _, filter := range filters {
			if shouldRun, forced, defaults := filter(presubmit); shouldRun {
				return shouldRun, forced, defaults
			}
		}
		return false, false, false
	}
}

// FilterPresubmits determines which presubmits should run and which should be skipped
// by evaluating the user-provided filter.
func FilterPresubmits(filter Filter, changes config.ChangedFilesProvider, branch string, presubmits []config.Presubmit, logger *logrus.Entry) ([]config.Presubmit, []config.Presubmit, error) {

	var toTrigger []config.Presubmit
	var namesToTrigger []string
	var toSkipSuperset []config.Presubmit
	for _, presubmit := range presubmits {
		matches, forced, defaults := filter(presubmit)
		if !matches {
			continue
		}
		shouldRun, err := presubmit.ShouldRun(branch, changes, forced, defaults)
		if err != nil {
			return nil, nil, err
		}
		if shouldRun {
			toTrigger = append(toTrigger, presubmit)
			namesToTrigger = append(namesToTrigger, presubmit.Name)
		} else {
			toSkipSuperset = append(toSkipSuperset, presubmit)
		}
	}

	toSkip := determineSkippedPresubmits(toTrigger, toSkipSuperset, logger)
	var namesToSkip []string
	for _, presubmit := range toSkip {
		namesToSkip = append(namesToSkip, presubmit.Name)
	}

	logger.WithFields(logrus.Fields{"to-trigger": namesToTrigger, "to-skip": namesToSkip}).Debugf("Filtered %d jobs, found %d to trigger and %d to skip.", len(presubmits), len(toTrigger), len(toSkipSuperset))
	return toTrigger, toSkip, nil
}

// determineSkippedPresubmits identifies the largest set of contexts we can actually
// post skipped contexts for, given a set of presubmits we're triggering. We don't
// want to skip a job that posts a context that will be written to by a job we just
// identified for triggering or the skipped context will override the triggered one
func determineSkippedPresubmits(toTrigger, toSkipSuperset []config.Presubmit, logger *logrus.Entry) []config.Presubmit {
	triggeredContexts := sets.NewString()
	for _, presubmit := range toTrigger {
		triggeredContexts.Insert(presubmit.Context)
	}
	var toSkip []config.Presubmit
	for _, presubmit := range toSkipSuperset {
		if triggeredContexts.Has(presubmit.Context) {
			logger.WithFields(logrus.Fields{"context": presubmit.Context, "job": presubmit.Name}).Debug("Not skipping job as context will be created by a triggered job.")
			continue
		}
		toSkip = append(toSkip, presubmit)
	}
	return toSkip
}

// RetestFilter builds a filter for `/retest`
func RetestFilter(failedContexts, allContexts sets.String) Filter {
	return func(p config.Presubmit) (bool, bool, bool) {
		return failedContexts.Has(p.Context) || (!p.NeedsExplicitTrigger() && !allContexts.Has(p.Context)), false, true
	}
}

type contextGetter func() (sets.String, sets.String, error)

// PresubmitFilter creates a filter for presubmits
func PresubmitFilter(honorOkToTest bool, contextGetter contextGetter, body string, logger *logrus.Entry) (Filter, error) {
	// the filters determine if we should check whether a job should run, whether
	// it should run regardless of whether its triggering conditions match, and
	// what the default behavior should be for that check. Multiple filters
	// can match a single presubmit, so it is important to order them correctly
	// as they have precedence -- filters that override the false default should
	// match before others. We order filters by amount of specificity.
	var filters []Filter
	filters = append(filters, CommandFilter(body))
	if RetestRe.MatchString(body) {
		logger.Debug("Using retest filter.")
		failedContexts, allContexts, err := contextGetter()
		if err != nil {
			return nil, err
		}
		filters = append(filters, RetestFilter(failedContexts, allContexts))
	}
	if (honorOkToTest && OkToTestRe.MatchString(body)) || TestAllRe.MatchString(body) {
		logger.Debug("Using test-all filter.")
		filters = append(filters, TestAllFilter())
	}
	return AggregateFilter(filters), nil
}
