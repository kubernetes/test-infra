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
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/andygrunwald/go-gerrit"

	"k8s.io/test-infra/prow/gerrit/client"
)

const (
	gerritServer = "http://localhost/fakegerritserver"
)

func makeTimeStamp(t time.Time) gerrit.Timestamp {
	return gerrit.Timestamp{Time: t}
}

type LastSyncState map[string]map[string]time.Time

func TestUnmarshall(t *testing.T) {
	var state LastSyncState
	//buf, _ := os.ReadFile("/usr/local/google/home/mpherman/Documents/touchtest/test")

	if err := json.Unmarshal([]byte(""), &state); err != nil {
		t.Fatal(err)
	}

	t.Fatalf("Unmarshalled fine: %v", state)
}

func TestGerrit(t *testing.T) {
	t.Parallel()

	startTime := time.Now().UTC()

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
		Updated:         makeTimeStamp(startTime),
		Branch:          "master",
		Status:          "NEW",
		Revisions:       map[string]client.RevisionInfo{},
		Messages:        []gerrit.ChangeMessageInfo{},
	}

	account := gerrit.AccountInfo{
		AccountID: 1,
		Name:      "Prow Bot",
		Username:  "testbot",
	}

	branch := gerrit.BranchInfo{}

	if err = addBranchToServer(branch, "test-infra", "master"); err != nil {
		t.Fatalf("failed to add branch to server: %v", err)
	}
	if err = addAccountToServer(account); err != nil {
		t.Fatalf("Failed to add change to server: %s", err)
	}
	if err = addChangeToServer(change, "fakegerritserver"); err != nil {
		t.Fatalf("Failed to add change to server: %s", err)
	}

	//Give some time for gerrit to pick up the change
	time.Sleep(1 * time.Minute)

	resp, err := gerritClient.GetChange(gerritServer, "1")
	if err != nil {
		reset()
		t.Errorf("Failed getting gerrit change: %v", err)
	}

	if len(resp.Messages) < 1 {
		t.Errorf("Original updated time was %s, and no messages have been added to change: %v", startTime, resp)
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

func login(id string) error {
	_, err := http.Get(fmt.Sprintf("http://localhost/fakegerritserver/admin/login/%s", id))
	if err != nil {
		return err
	}
	return nil
}

func addChangeToServer(change gerrit.ChangeInfo, project string) error {
	body, err := json.Marshal(change)
	if err != nil {
		return err
	}

	_, err = http.Post(fmt.Sprintf("http://localhost/fakegerritserver/admin/add/change/%s", project), "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	return nil
}

func addAccountToServer(account gerrit.AccountInfo) error {
	body, err := json.Marshal(account)
	if err != nil {
		return err
	}

	_, err = http.Post("http://localhost/fakegerritserver/admin/add/account", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	return nil

}

func addBranchToServer(branch gerrit.BranchInfo, project, name string) error {
	body, err := json.Marshal(branch)
	if err != nil {
		return err
	}

	_, err = http.Post(fmt.Sprintf("http://localhost/fakegerritserver/admin/add/branch/%s/%s", project, name), "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	return nil
}
