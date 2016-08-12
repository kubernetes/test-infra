/*
Copyright 2016 The Kubernetes Authors All rights reserved.

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

package event

import (
	"testing"
	"time"

	"github.com/google/go-github/github"
)

func getDate(year int, month time.Month, day, hour, min, sec int) *time.Time {
	date := time.Date(year, month, day, hour, min, sec, 0, time.UTC)
	return &date
}

func TestCreationBefore(t *testing.T) {
	if CreatedBefore(
		time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC),
	).Match(nil) {
		t.Error("Shouldn't match nil event")
	}
	if CreatedBefore(
		time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC),
	).Match(&github.IssueEvent{}) {
		t.Error("Shouldn't match nil CreatedAt")
	}
	if CreatedBefore(
		time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC),
	).Match(&github.IssueEvent{
		CreatedAt: getDate(2000, 1, 1, 12, 0, 1),
	}) {
		t.Error("Shouldn't match later event")
	}
	if !CreatedBefore(
		time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC),
	).Match(&github.IssueEvent{
		CreatedAt: getDate(2000, 1, 1, 11, 0, 0),
	}) {
		t.Error("Should match earlier event")
	}
}

func TestCreationAfter(t *testing.T) {
	if CreatedAfter(
		time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC),
	).Match(nil) {
		t.Error("Shouldn't match nil event")
	}
	if CreatedAfter(
		time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC),
	).Match(&github.IssueEvent{}) {
		t.Error("Shouldn't match nil CreatedAt")
	}
	if !CreatedAfter(
		time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC),
	).Match(&github.IssueEvent{
		CreatedAt: getDate(2000, 1, 1, 12, 0, 1),
	}) {
		t.Error("Should match later event")
	}
	if CreatedAfter(
		time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC),
	).Match(&github.IssueEvent{
		CreatedAt: getDate(2000, 1, 1, 11, 0, 0),
	}) {
		t.Error("Shouldn't match earlier event")
	}
}

func makeEventWithActor(actor string) *github.IssueEvent {
	return &github.IssueEvent{
		Actor: &github.User{Login: &actor},
	}
}

func TestActor(t *testing.T) {
	if Actor("actor").Match(nil) {
		t.Error("Shouldn't match nil event")
	}
	if Actor("actor").Match(&github.IssueEvent{}) {
		t.Error("Shouldn't match nil Actor")
	}
	if Actor("actor").Match(&github.IssueEvent{Actor: &github.User{}}) {
		t.Error("Shouldn't match nil Login")
	}
	if Actor("actor").Match(makeEventWithActor("user")) {
		t.Error("Shouldn't match actor with different names")
	}
	if !Actor("actor").Match(makeEventWithActor("actor")) {
		t.Error("Should match actor with similar name")
	}
	if !Actor("actoR").Match(makeEventWithActor("Actor")) {
		t.Error("Should match actor with similar name, but different case")
	}
}
