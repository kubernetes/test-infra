/*
Copyright 2020 The Kubernetes Authors.

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
	"reflect"
	"testing"
	"time"

	"github.com/andygrunwald/go-gerrit"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/apimachinery/pkg/util/sets"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/gerrit/client"
)

func TestPresubmitContexts(t *testing.T) {
	jobs := func(names ...string) []config.Presubmit {
		var presubmits []config.Presubmit
		for _, n := range names {
			var p config.Presubmit
			p.Name = n
			presubmits = append(presubmits, p)
		}
		return presubmits
	}
	cases := []struct {
		name       string
		presubmits []config.Presubmit
		failing    sets.String
		failed     sets.String
		all        sets.String
	}{
		{
			name: "basically works",
		},
		{
			name:       "simple case works",
			presubmits: jobs("hello-fail", "world"),
			failing:    sets.NewString("world"),
			failed:     sets.NewString("world"),
			all:        sets.NewString("hello-fail", "world"),
		},
		{
			name:       "ignore failures from deleted jobs",
			presubmits: jobs("failing", "passing"),
			failing:    sets.NewString("failing", "deleted"),
			failed:     sets.NewString("failing"),
			all:        sets.NewString("failing", "passing"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotFailed, gotAll := presubmitContexts(tc.failing, tc.presubmits, logrus.WithField("case", tc.name))
			if !equality.Semantic.DeepEqual(tc.failed, gotFailed) {
				t.Errorf("wrong failures:%s", diff.ObjectReflectDiff(tc.failed, gotFailed))
			}
			if !equality.Semantic.DeepEqual(tc.all, gotAll) {
				t.Errorf("wrong all contexts:%s", diff.ObjectReflectDiff(tc.all, gotAll))
			}
		})
	}
}

func stamp(t time.Time) gerrit.Timestamp {
	return gerrit.Timestamp{Time: t}
}

func TestCurrentMessages(t *testing.T) {
	now := time.Now()
	before := now.Add(-time.Minute)
	after := now.Add(time.Hour)
	later := after.Add(time.Hour)

	now3 := gerrit.ChangeMessageInfo{
		RevisionNumber: 3,
		Date:           stamp(now),
		Message:        "now",
	}
	after3 := gerrit.ChangeMessageInfo{
		RevisionNumber: 3,
		Date:           stamp(after),
		Message:        "after",
	}
	later3 := gerrit.ChangeMessageInfo{
		RevisionNumber: 3,
		Date:           stamp(later),
		Message:        "later",
	}
	after4 := gerrit.ChangeMessageInfo{
		RevisionNumber: 4,
		Date:           stamp(after),
		Message:        "4-after",
	}

	cases := []struct {
		name   string
		change gerrit.ChangeInfo
		since  time.Time
		want   []gerrit.ChangeMessageInfo
	}{
		{
			name: "basically works",
		},
		{
			name:  "simple case",
			since: before,
			change: gerrit.ChangeInfo{
				Revisions: map[string]gerrit.RevisionInfo{
					"3": {
						Number: 3,
					},
				},
				CurrentRevision: "3",
				Messages:        []gerrit.ChangeMessageInfo{now3, after3, later3},
			},
			want: []gerrit.ChangeMessageInfo{now3, after3, later3},
		},
		{
			name:  "reject old messages",
			since: now,
			change: gerrit.ChangeInfo{
				Revisions: map[string]gerrit.RevisionInfo{
					"3": {
						Number: 3,
					},
				},
				CurrentRevision: "3",
				Messages:        []gerrit.ChangeMessageInfo{now3, after3, later3},
			},
			want: []gerrit.ChangeMessageInfo{after3, later3},
		},
		{
			name:  "reject message from other revisions",
			since: before,
			change: gerrit.ChangeInfo{
				Revisions: map[string]gerrit.RevisionInfo{
					"3": {
						Number: 3,
					},
				},
				CurrentRevision: "3",
				Messages:        []gerrit.ChangeMessageInfo{now3, after4, later3},
			},
			want: []gerrit.ChangeMessageInfo{now3, later3},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := currentMessages(tc.change, tc.since)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("wrong messages:%s", diff.ObjectReflectDiff(got, tc.want))
			}
		})
	}
}

func TestMessageFilter(t *testing.T) {
	old := time.Now().Add(-1 * time.Hour)
	older := old.Add(-1 * time.Hour)
	job := func(name string, patch func(j *config.Presubmit)) config.Presubmit {
		var presubmit config.Presubmit
		presubmit.Name = name
		presubmit.Context = name
		presubmit.Trigger = config.DefaultTriggerFor(name)
		presubmit.RerunCommand = config.DefaultRerunCommandFor(name)
		presubmit.AlwaysRun = true
		if patch != nil {
			patch(&presubmit)
		}
		return presubmit
	}
	msg := func(content string, t time.Time) gerrit.ChangeMessageInfo {
		return gerrit.ChangeMessageInfo{Message: content, Date: gerrit.Timestamp{Time: t}}
	}
	type check struct {
		job             config.Presubmit
		shouldRun       bool
		forcedToRun     bool
		defaultBehavior bool
		triggered       time.Time
	}
	cases := []struct {
		name     string
		messages []gerrit.ChangeMessageInfo
		failed   sets.String
		all      sets.String
		checks   []check
	}{
		{
			name: "basically works",
		},
		{
			name:     "/test foo works",
			messages: []gerrit.ChangeMessageInfo{msg("/test foo", older), msg("/test bar", old)},
			all:      sets.NewString("foo", "bar", "ignored"),
			checks: []check{
				{
					job:             job("foo", nil),
					shouldRun:       true,
					forcedToRun:     true,
					defaultBehavior: true,
					triggered:       older,
				},
				{
					job:             job("bar", nil),
					shouldRun:       true,
					forcedToRun:     true,
					defaultBehavior: true,
					triggered:       old,
				},
				{
					job:             job("ignored", nil),
					shouldRun:       false,
					forcedToRun:     false,
					defaultBehavior: false,
				},
			},
		},
		{
			name:     "/test all triggers multiple",
			messages: []gerrit.ChangeMessageInfo{msg("/test all", old)},
			all:      sets.NewString("foo", "bar"),
			checks: []check{
				{
					job:             job("foo", nil),
					shouldRun:       true,
					forcedToRun:     false,
					defaultBehavior: false,
					triggered:       old,
				},
				{
					job:             job("bar", nil),
					shouldRun:       true,
					forcedToRun:     false,
					defaultBehavior: false,
					triggered:       old,
				},
			},
		},
		{
			name:     "/retest triggers failures",
			messages: []gerrit.ChangeMessageInfo{msg("/retest", old)},
			failed:   sets.NewString("failed"),
			all:      sets.NewString("foo", "bar", "failed"),
			checks: []check{
				{
					job:             job("foo", nil),
					shouldRun:       false,
					forcedToRun:     false,
					defaultBehavior: false,
				},
				{
					job:             job("failed", nil),
					shouldRun:       true,
					forcedToRun:     false,
					defaultBehavior: true,
					triggered:       old,
				},
				{
					job:             job("bar", nil),
					shouldRun:       false,
					forcedToRun:     false,
					defaultBehavior: false,
				},
			},
		},
		{
			name:     "draft->active by clicking `MARK AS ACTIVE` triggers multiple",
			messages: []gerrit.ChangeMessageInfo{msg(client.ReadyForReviewMessageFixed, old)},
			all:      sets.NewString("foo", "bar"),
			checks: []check{
				{
					job:             job("foo", nil),
					shouldRun:       true,
					forcedToRun:     false,
					defaultBehavior: false,
					triggered:       old,
				},
				{
					job:             job("bar", nil),
					shouldRun:       true,
					forcedToRun:     false,
					defaultBehavior: false,
					triggered:       old,
				},
			},
		},
		{
			name: "draft->active by clicking `SEND AND START REVIEW` triggers multiple",
			messages: []gerrit.ChangeMessageInfo{msg(`Patch Set 1:

			(1 comment)
			
			`+client.ReadyForReviewMessageCustomizable, old)},
			all: sets.NewString("foo", "bar"),
			checks: []check{
				{
					job:             job("foo", nil),
					shouldRun:       true,
					forcedToRun:     false,
					defaultBehavior: false,
					triggered:       old,
				},
				{
					job:             job("bar", nil),
					shouldRun:       true,
					forcedToRun:     false,
					defaultBehavior: false,
					triggered:       old,
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logger := logrus.WithField("case", tc.name)
			triggerTimes := map[string]time.Time{}
			filt := messageFilter(tc.messages, tc.failed, tc.all, triggerTimes, logger)
			for _, check := range tc.checks {
				t.Run(check.job.Name, func(t *testing.T) {
					fixed := []config.Presubmit{check.job}
					config.SetPresubmitRegexes(fixed)
					check.job = fixed[0]
					shouldRun, forcedToRun, defaultBehavior := filt.ShouldRun(check.job)
					if got, want := shouldRun, check.shouldRun; got != want {
						t.Errorf("shouldRun: got %t, want %t", got, want)
					}
					if got, want := forcedToRun, check.forcedToRun; got != want {
						t.Errorf("forcedToRun: got %t, want %t", got, want)
					}
					if got, want := defaultBehavior, check.defaultBehavior; got != want {
						t.Errorf("defaultBehavior: got %t, want %t", got, want)
					}
				})
			}
			// Validate that triggerTimes was populated correctly after ShouldRun is called on every job.
			for _, check := range tc.checks {
				if !triggerTimes[check.job.Name].Equal(check.triggered) {
					t.Errorf("expected job %q to have trigger time %v, but got %v", check.job.Name, check.triggered, triggerTimes[check.job.Name])
				}
			}
		})
	}
}
