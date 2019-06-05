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

package bugzilla

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/diff"
)

var (
	bugData   = []byte(`{"bugs":[{"alias":[],"assigned_to":"Steve Kuznetsov","assigned_to_detail":{"email":"skuznets","id":381851,"name":"skuznets","real_name":"Steve Kuznetsov"},"blocks":[],"cc":["Sudha Ponnaganti"],"cc_detail":[{"email":"sponnaga","id":426940,"name":"sponnaga","real_name":"Sudha Ponnaganti"}],"classification":"Red Hat","component":["Test Infrastructure"],"creation_time":"2019-05-01T19:33:36Z","creator":"Dan Mace","creator_detail":{"email":"dmace","id":330250,"name":"dmace","real_name":"Dan Mace"},"deadline":null,"depends_on":[],"docs_contact":"","dupe_of":null,"groups":[],"id":1705243,"is_cc_accessible":true,"is_confirmed":true,"is_creator_accessible":true,"is_open":true,"keywords":[],"last_change_time":"2019-05-17T15:13:13Z","op_sys":"Unspecified","platform":"Unspecified","priority":"unspecified","product":"OpenShift Container Platform","qa_contact":"","resolution":"","see_also":[],"severity":"medium","status":"VERIFIED","summary":"[ci] cli image flake affecting *-images jobs","target_milestone":"---","target_release":["3.11.z"],"url":"","version":["3.11.0"],"whiteboard":""}],"faults":[]}`)
	bugStruct = &Bug{Alias: []int{}, AssignedTo: "Steve Kuznetsov", AssignedToDetail: &User{Email: "skuznets", ID: 381851, Name: "skuznets", RealName: "Steve Kuznetsov"}, Blocks: []int{}, CC: []string{"Sudha Ponnaganti"}, CCDetail: []User{{Email: "sponnaga", ID: 426940, Name: "sponnaga", RealName: "Sudha Ponnaganti"}}, Classification: "Red Hat", Component: []string{"Test Infrastructure"}, CreationTime: "2019-05-01T19:33:36Z", Creator: "Dan Mace", CreatorDetail: &User{Email: "dmace", ID: 330250, Name: "dmace", RealName: "Dan Mace"}, DependsOn: []int{}, ID: 1705243, IsCCAccessible: true, IsConfirmed: true, IsCreatorAccessible: true, IsOpen: true, Groups: []string{}, Keywords: []string{}, LastChangeTime: "2019-05-17T15:13:13Z", OperatingSystem: "Unspecified", Platform: "Unspecified", Priority: "unspecified", Product: "OpenShift Container Platform", SeeAlso: []string{}, Severity: "medium", Status: "VERIFIED", Summary: "[ci] cli image flake affecting *-images jobs", TargetRelease: []string{"3.11.z"}, TargetMilestone: "---", Version: []string{"3.11.0"}}
)

func clientForUrl(url string) *client {
	return &client{
		logger:   logrus.WithField("testing", "true"),
		endpoint: url,
		client: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		},
		getAPIKey: func() []byte {
			return []byte("api-key")
		},
	}
}

func TestGetBug(t *testing.T) {
	testServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-BUGZILLA-API-KEY") != "api-key" {
			t.Error("did not get api-key passed in X-BUGZILLA-API-KEY header")
			http.Error(w, "403 Forbidden", http.StatusForbidden)
			return
		}
		if r.URL.Query().Get("api_key") != "api-key" {
			t.Error("did not get api-key passed in api_key query parameter")
			http.Error(w, "403 Forbidden", http.StatusForbidden)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/rest/bug/") {
			t.Errorf("incorrect path to get a bug: %s", r.URL.Path)
			http.Error(w, "400 Bad Request", http.StatusBadRequest)
			return
		}
		if id, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/rest/bug/")); err != nil {
			t.Errorf("malformed bug id: %s", r.URL.Path)
			http.Error(w, "400 Bad Request", http.StatusBadRequest)
			return
		} else {
			if id == 1705243 {
				w.Write(bugData)
			} else {
				http.Error(w, "404 Not Found", http.StatusNotFound)
			}
		}
	}))
	defer testServer.Close()
	client := clientForUrl(testServer.URL)

	// this should give us what we want
	bug, err := client.GetBug(1705243)
	if err != nil {
		t.Errorf("expected no error, but got one: %v", err)
	}
	if !reflect.DeepEqual(bug, bugStruct) {
		t.Errorf("got incorrect bug: %v", diff.ObjectReflectDiff(bug, bugStruct))
	}

	// this should 404
	otherBug, err := client.GetBug(1)
	if err == nil {
		t.Error("expected an error, but got none")
	} else if !IsNotFound(err) {
		t.Errorf("expected a not found error, got %v", err)
	}
	if otherBug != nil {
		t.Errorf("expected no bug, got: %v", otherBug)
	}
}
