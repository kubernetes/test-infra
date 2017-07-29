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

package comment

import (
	"testing"
	"time"
)

func timeAgo(d time.Duration) *time.Time {
	t := time.Now().Add(-d)
	return &t
}

func makeComment(body, author string, beforeNow time.Duration) *Comment {
	return &Comment{
		Body:      &body,
		Author:    &author,
		CreatedAt: timeAgo(beforeNow),
	}
}

func TestMaxReachNotReachedNoStart(t *testing.T) {
	comments := []*Comment{
		makeComment("[SOMETHING] Something", "k8s-merge-robot", 10*time.Hour),
		makeComment("[NOTIF] Notification", "k8s-merge-robot", 10*time.Hour),
	}

	pinger := NewPinger("NOTIF", "k8s-merge-robot").SetMaxCount(2)

	if pinger.IsMaxReached(comments, nil) {
		t.Error("Should not have reached the maximum")
	}
}

func TestMaxReachNotReachedWithStart(t *testing.T) {
	comments := []*Comment{
		makeComment("[NOTIF] Notification", "k8s-merge-robot", 14*time.Hour),
		makeComment("[NOTIF] Notification", "k8s-merge-robot", 12*time.Hour),
		makeComment("[SOMETHING] Something", "k8s-merge-robot", 10*time.Hour),
		makeComment("[NOTIF] Notification", "k8s-merge-robot", 10*time.Hour),
	}

	pinger := NewPinger("NOTIF", "k8s-merge-robot").SetMaxCount(2)

	if pinger.IsMaxReached(comments, timeAgo(11*time.Hour)) {
		t.Error("Should not have reached the maximum")
	}
}

func TestMaxReachNoLimit(t *testing.T) {
	comments := []*Comment{
		makeComment("[NOTIF] Notification", "k8s-merge-robot", 14*time.Hour),
		makeComment("[NOTIF] Notification", "k8s-merge-robot", 12*time.Hour),
		makeComment("[SOMETHING] Something", "k8s-merge-robot", 10*time.Hour),
		makeComment("[NOTIF] Notification", "k8s-merge-robot", 10*time.Hour),
	}

	pinger := NewPinger("NOTIF", "k8s-merge-robot")

	if pinger.IsMaxReached(comments, nil) {
		t.Error("Should not have reached the non-existing maximum")
	}
}

func TestNotification(t *testing.T) {
	comments := []*Comment{
		makeComment("[SOMETHING] Something", "k8s-merge-robot", 10*time.Hour),
	}

	notif := NewPinger("NOTIF", "k8s-merge-robot").SetDescription("Description").PingNotification(comments, "who", nil)
	if notif == nil {
		t.Error("PingNotification should have created a notif")
	}
	expectedNotif := Notification{Name: "NOTIF", Arguments: "who", Context: "Description"}
	if *notif != expectedNotif {
		t.Error(*notif, "doesn't match expectedNotif:", expectedNotif)
	}
}

func TestNotificationNilTimePeriod(t *testing.T) {
	comments := []*Comment{
		makeComment("[NOTIF] Notification", "k8s-merge-robot", 14*time.Hour),
		makeComment("[NOTIF] Notification", "k8s-merge-robot", 13*time.Hour),
		makeComment("[NOTIF] Notification", "k8s-merge-robot", 12*time.Hour),
		makeComment("[NOTIF] Notification", "k8s-merge-robot", 11*time.Hour),
		makeComment("[SOMETHING] Something", "k8s-merge-robot", 10*time.Hour),
	}

	notif := NewPinger("NOTIF", "k8s-merge-robot").PingNotification(comments, "who", nil)
	if notif == nil {
		t.Error("PingNotification should have created a notif")
	}
}

func TestNotificationTimePeriodNotReached(t *testing.T) {
	comments := []*Comment{
		makeComment("[NOTIF] Notification", "k8s-merge-robot", 5*time.Hour),
		makeComment("[NOTIF] Notification", "k8s-merge-robot", 3*time.Hour),
		makeComment("[NOTIF] Notification", "k8s-merge-robot", 1*time.Hour),
	}

	notif := NewPinger("NOTIF", "k8s-merge-robot").SetTimePeriod(2*time.Hour).PingNotification(comments, "who", nil)
	if notif != nil {
		t.Error("PingNotification shouldn't have created a notif")
	}
}

func TestNotificationTimePeriodReached(t *testing.T) {
	comments := []*Comment{
		makeComment("[NOTIF] Notification", "k8s-merge-robot", 4*time.Hour),
		makeComment("[NOTIF] Notification", "k8s-merge-robot", 3*time.Hour),
		makeComment("[NOTIF] Notification", "k8s-merge-robot", 2*time.Hour),
	}

	notif := NewPinger("NOTIF", "k8s-merge-robot").SetTimePeriod(time.Hour).PingNotification(comments, "who", nil)
	if notif == nil {
		t.Error("PingNotification should have created a notif")
	}
}

func TestNotificationStartDate(t *testing.T) {
	comments := []*Comment{}
	notif := NewPinger("NOTIF", "k8s-merge-robot").SetTimePeriod(10*time.Hour).PingNotification(comments, "who", timeAgo(2*time.Hour))
	if notif != nil {
		t.Error("PingNotification shouldn't have created a notif")
	}
}
