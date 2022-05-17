/*
Copyright 2022 The Kubernetes Authors.

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

package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/andygrunwald/go-gerrit"

	"k8s.io/test-infra/prow/gerrit/client"
)

const (
	gerritServer = "http://localhost/fakegerritserver"
)

var (
	timeNow       = time.Date(2022, time.May, 15, 1, 2, 3, 4, time.UTC)
	timeLast      = time.Date(2000, time.May, 15, 1, 2, 3, 4, time.UTC)
	lastSyncState = client.LastSyncState{"http://localhost/fakegerritserver": map[string]time.Time{"fakegerritserver": timeLast}}
)

func makeTimeStamp(t time.Time) gerrit.Timestamp {
	return gerrit.Timestamp{Time: t}
}

func TestGerrit(t *testing.T) {
	t.Parallel()

	gerritClient, err := client.NewClient(map[string][]string{gerritServer: {"fakegerritserver"}})
	if err != nil {
		reset()
		t.Fatalf("Failed creating gerritClient: %v", err)
	}

	change := gerrit.ChangeInfo{
		CurrentRevision: "1",
		ID:              "1",
		ChangeID:        "1",
		Project:         "test-infra",
		Branch:          "master",
		Status:          "NEW",
		Updated:         makeTimeStamp(timeNow),
		Revisions: map[string]client.RevisionInfo{
			"1": {
				Number:  1,
				Created: makeTimeStamp(timeNow.Add(-time.Hour)),
			},
		},
		Messages: []gerrit.ChangeMessageInfo{
			{
				Message:        "Hello",
				RevisionNumber: 1,
				Date:           makeTimeStamp(timeNow),
			},
		},
	}

	err = addChangeToServer(change)
	if err != nil {
		reset()
		t.Fatalf("Failed to add change to server: %s", err)
	}

	resp, err := gerritClient.GetChange(gerritServer, "1")
	if err != nil {
		reset()
		t.Errorf("Failed getting gerrit change: %v", err)
	}
	if resp.ChangeID != "1" {
		reset()
		t.Errorf("Did not return expected ChangeID. Want: %q, got: %q", "1", resp.ChangeID)
	}

	changes := gerritClient.QueryChanges(lastSyncState, 10)
	if len(changes[gerritServer]) != 1 {
		reset()
		t.Errorf("Did not return expected ChangeID. Want: %q, got: %v", "1", len(changes[gerritServer]))
	}

	// Reset the fakeGerritServer so the test can be run again
	reset()
}

func reset() error {
	_, err := http.Get("http://localhost/fakegerritserver/admin/reset")
	if err != nil {
		return err
	}
	return nil
}

func addChangeToServer(change gerrit.ChangeInfo) error {
	body, err := json.Marshal(change)
	if err != nil {
		return err
	}

	_, err = http.Post("http://localhost/fakegerritserver/admin/add", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	return nil
}
