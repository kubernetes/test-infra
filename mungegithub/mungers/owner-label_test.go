/*
Copyright 2017 The Kubernetes Authors.

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

package mungers

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	github_util "k8s.io/test-infra/mungegithub/github"
	github_test "k8s.io/test-infra/mungegithub/github/testing"

	"github.com/google/go-github/github"
	"k8s.io/apimachinery/pkg/util/sets"
)

type testOwnerLabeler struct{}

func (t testOwnerLabeler) AllPossibleOwnerLabels() sets.String {
	return sets.NewString()
}

func (t testOwnerLabeler) FindLabelsForPath(path string) sets.String {
	if strings.Contains(path, "docs") {
		return sets.NewString("kind/design")
	}
	return sets.NewString()
}

func TestOwnerLabelMunge(t *testing.T) {
	const testBotName = "dummy"
	tests := []struct {
		files       []*github.CommitFile
		events      []*github.IssueEvent
		mustHave    []string
		mustNotHave []string
	}{
		{
			files:       commitFiles([]string{"docs/proposals"}),
			events:      BotAddedDesign(testBotName),
			mustHave:    []string{"kind/design"},
			mustNotHave: []string{},
		},
	}
	for testNum, test := range tests {
		client, server, mux := github_test.InitServer(t, docsProposalIssue(testBotName), ValidPR(), test.events, nil, nil, nil, test.files)
		mux.HandleFunc("/repos/o/r/issues/1/labels/kind/design", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte{})
		})
		mux.HandleFunc("/repos/o/r/issues/1/labels", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			out := []github.Label{{}}
			data, err := json.Marshal(out)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			w.Write(data)

		})

		config := &github_util.Config{
			Org:     "o",
			Project: "r",
		}
		config.SetClient(client)
		config.BotName = testBotName

		o := OwnerLabelMunger{labeler: testOwnerLabeler{}}

		obj, err := config.GetObject(1)
		if err != nil {
			t.Fatalf("%v", err)
		}

		o.Munge(obj)

		for _, l := range test.mustHave {
			if !obj.HasLabel(l) {
				t.Errorf("%d: Did not find label %q, labels: %v", testNum, l, obj.Issue.Labels)
			}
		}
		for _, l := range test.mustNotHave {
			if obj.HasLabel(l) {
				t.Errorf("%d: Found label %q and should not have, labels: %v", testNum, l, obj.Issue.Labels)
			}
		}
		server.Close()
	}
}
