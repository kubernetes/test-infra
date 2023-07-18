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
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/gerrit/client"
)

const (
	gerritServer = "http://localhost/fakegerritserver"
)

type LastSyncState map[string]map[string]time.Time

func TestGerrit(t *testing.T) {
	startTime := gerrit.Timestamp{Time: time.Now().AddDate(0, 0, 2).UTC()}
	tests := []struct {
		name             string
		change           gerrit.ChangeInfo
		expectedMessages []string
	}{
		{
			name: "1 New change with 1 presubit triggered",
			change: gerrit.ChangeInfo{
				CurrentRevision: "1",
				ID:              "1",
				ChangeID:        "1",
				Project:         "gerrit-test-infra-0",
				Updated:         startTime,
				Branch:          "master",
				Status:          "NEW",
				Revisions:       map[string]client.RevisionInfo{"1": {Number: 1, Ref: "refs/changes/00/1/1", Created: startTime}},
				Messages:        []gerrit.ChangeMessageInfo{{RevisionNumber: 1, Message: "/test all", ID: "1", Date: startTime}},
			},
			expectedMessages: []string{"/test all", "Triggered 1 prow jobs (0 suppressed reporting): \n  * Name: hello-world-presubmit"},
		},
		{
			name: "no presubmit Prow jobs automatically triggered from WorkInProgess change",
			change: client.ChangeInfo{
				CurrentRevision: "1",
				ChangeID:        "2",
				ID:              "2",
				Project:         "gerrit-test-infra-1",
				Status:          "NEW",
				Branch:          "master",
				Updated:         startTime,
				WorkInProgress:  true,
				Revisions: map[string]gerrit.RevisionInfo{
					"1": {
						Number: 1001,
					},
				},
			},
			expectedMessages: []string{},
		},
		{
			name: "presubmit runs when a file matches run_if_changed",
			change: client.ChangeInfo{
				CurrentRevision: "1",
				ChangeID:        "3",
				ID:              "3",
				Branch:          "master",
				Project:         "gerrit-test-infra-2",
				Updated:         startTime,
				Status:          "NEW",
				Messages:        []gerrit.ChangeMessageInfo{{RevisionNumber: 1, Message: "/test all", ID: "1", Date: startTime}},
				Revisions: map[string]client.RevisionInfo{
					"1": {
						Number: 1001,
						Files: map[string]client.FileInfo{
							"bee-movie-script.txt": {},
							"africa-lyrics.txt":    {},
							"important-code.go":    {},
						},
						Created: startTime,
					},
				},
			},
			expectedMessages: []string{"/test all", "Triggered 2 prow jobs (0 suppressed reporting): \n  * Name: hello-world-presubmit\n  * Name: bee-movie-presubmit"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			gerritClient, err := client.NewClient(map[string]map[string]*config.GerritQueryFilter{gerritServer: {"fakegerritserver": nil}}, 0, 0)
			if err != nil {
				t.Fatalf("Failed creating gerritClient: %v", err)
			}

			account := gerrit.AccountInfo{
				AccountID: 1,
				Name:      "Prow Bot",
				Username:  "testbot",
			}

			branch := gerrit.BranchInfo{}

			if err = addBranchToServer(branch, tc.change.Project, "master"); err != nil {
				t.Fatalf("failed to add branch to server: %v", err)
			}
			if err = addAccountToServer(account); err != nil {
				t.Fatalf("Failed to add change to server: %s", err)
			}
			if err = login(account.AccountID); err != nil {
				t.Fatalf("Failed to set self on server: %s", err)
			}
			if err = addChangeToServer(tc.change, tc.change.Project); err != nil {
				t.Fatalf("Failed to add change to server: %s", err)

			}

			//Give some time for gerrit to pick up the change
			wait.Poll(5*time.Second, 20*time.Second, expectedMessagesRecievedFunc(gerritClient, tc.change.ChangeID, tc.expectedMessages))

			resp, err := gerritClient.GetChange(gerritServer, tc.change.ChangeID)
			if err != nil {
				reset()
				t.Errorf("Failed getting gerrit change: %v", err)
			}

			if diff := cmp.Diff(tc.expectedMessages, mapToStrings(resp.Messages), cmpopts.SortSlices(func(a, b string) bool {
				return a < b
			})); diff != "" {
				t.Errorf("change message mismatch. want(-), got(+):\n%s", diff)
			}

			// Reset the fakeGerritServer so the test can be run again
			reset()
		})
	}

}

func expectedMessagesRecievedFunc(gerritClient *client.Client, ChangeID string, expectedMessages []string) func() (bool, error) {
	return func() (bool, error) {
		resp, err := gerritClient.GetChange(gerritServer, ChangeID)
		if err != nil {
			return false, nil
		}

		if diff := cmp.Diff(expectedMessages, mapToStrings(resp.Messages), cmpopts.SortSlices(func(a, b string) bool {
			return a < b
		})); diff != "" {
			return false, nil
		}
		return true, nil
	}
}

func mapToStrings(messages []gerrit.ChangeMessageInfo) []string {
	res := []string{}
	for _, msg := range messages {
		res = append(res, msg.Message)
	}
	return res
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
