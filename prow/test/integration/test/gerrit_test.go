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
	"strings"
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

func TestGerrit(t *testing.T) {
	startTime := time.Now().AddDate(0, 0, 2).UTC()

	gerritClient, err := client.NewClient(map[string][]string{gerritServer: {"fakegerritserver"}})
	if err != nil {
		t.Fatalf("Failed creating gerritClient: %v", err)
	}

	change := gerrit.ChangeInfo{
		CurrentRevision: "1",
		ID:              "1",
		ChangeID:        "1",
		Project:         "gerrit-test-infra",
		Updated:         makeTimeStamp(startTime),
		Branch:          "master",
		Status:          "NEW",
		Revisions:       map[string]client.RevisionInfo{"1": {Number: 1, Ref: "refs/changes/00/1/1", Created: makeTimeStamp(time.Now().AddDate(0, 0, 2).UTC())}},
		Messages:        []gerrit.ChangeMessageInfo{{RevisionNumber: 1, Message: "/test all", ID: "1", Date: makeTimeStamp(time.Now().AddDate(0, 0, 2).UTC())}},
	}

	account := gerrit.AccountInfo{
		AccountID: 1,
		Name:      "Prow Bot",
		Username:  "testbot",
	}

	branch := gerrit.BranchInfo{}

	if err = addBranchToServer(branch, "gerrit-test-infra", "master"); err != nil {
		t.Fatalf("failed to add branch to server: %v", err)
	}
	if err = addAccountToServer(account); err != nil {
		t.Fatalf("Failed to add change to server: %s", err)
	}
	if err = login(account.AccountID); err != nil {
		t.Fatalf("Failed to set self on server: %s", err)
	}
	if err = addChangeToServer(change, "gerrit-test-infra"); err != nil {
		t.Fatalf("Failed to add change to server: %s", err)
	}

	//Give some time for gerrit to pick up the change
	time.Sleep(15 * time.Second)

	resp, err := gerritClient.GetChange(gerritServer, "1")
	if err != nil {
		reset()
		t.Fatalf("Failed getting gerrit change: %v", err)
	}

	if len(resp.Messages) < 2 {
		t.Errorf("gerrit did not add any messages to change: %v", resp)
	}
	if !strings.Contains(resp.Messages[1].Message, "Triggered 1 prow jobs") {
		t.Errorf("Did not trigger prowjob. Message: %s", resp.Messages[1].Message)
	}

	// Reset the fakeGerritServer so the test can be run again
	reset()
}

func reset() error {
	_, err := http.Get(fmt.Sprintf("%s/admin/reset", gerritServer))
	if err != nil {
		return err
	}
	return nil
}

func login(id int) error {
	_, err := http.Get(fmt.Sprintf("%s/admin/login/%d", gerritServer, id))
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

	_, err = http.Post(fmt.Sprintf("%s/admin/add/change/%s", gerritServer, project), "application/json", bytes.NewReader(body))
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

	_, err = http.Post(fmt.Sprintf("%s/admin/add/account", gerritServer), "application/json", bytes.NewReader(body))
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

	_, err = http.Post(fmt.Sprintf("%s/admin/add/branch/%s/%s", gerritServer, project, name), "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	return nil
}
