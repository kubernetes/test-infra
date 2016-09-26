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

package github

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequest(t *testing.T) {
	c := NewClient("token")
	r := strings.NewReader("data here")
	req, err := c.request(http.MethodPost, "path/to/place", r)
	if err != nil {
		t.Errorf("Didn't expect error creating request: %s", err)
	}
	if req.Method != "POST" {
		t.Errorf("Wrong method. Got %s, expected POST", req.Method)
	}
	expectedURL := "https://api.github.com/path/to/place"
	if req.URL.String() != expectedURL {
		t.Errorf("Wrong URL. Got %s, expected %s", req.URL.String(), expectedURL)
	}
	auth := req.Header.Get("Authorization")
	if auth != "Token token" {
		t.Errorf("Wrong auth header. Got \"%s\", expected \"Token token\"", auth)
	}
}

func TestIsMember(t *testing.T) {
	// If the token is "member" then the requester is treated as a member.
	// The user "mem" is a member.
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ps := strings.Split(r.URL.Path, "/")
		// Validate the path.
		if len(ps) != 5 || ps[0] != "" || ps[1] != "orgs" || ps[3] != "members" {
			http.Error(w, "405 Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		user := ps[4]
		if ps[2] != "k8s" {
			http.Error(w, "302 Found", http.StatusFound)
			return
		}
		if r.Header.Get("Authorization") != "Token member" {
			if user == "req" {
				http.Error(w, "404 Not Found", http.StatusNotFound)
				return
			} else {
				http.Error(w, "302 Found", http.StatusFound)
				return
			}
		}
		if user == "mem" || user == "req" {
			http.Error(w, "204 No Content", http.StatusNoContent)
			return
		} else {
			http.Error(w, "404 Not Found", http.StatusNotFound)
			return
		}
	}))
	defer ts.Close()
	var testcases = []struct {
		description string
		org         string
		user        string
		reqMember   bool

		expectedMember bool
		expectedErr    bool
	}{
		{
			"Org that requester is not a member of",
			"otherorg",
			"mem",
			true,
			false,
			true,
		},
		{
			"Non-member requester looking up themself",
			"k8s",
			"req",
			false,
			false,
			false,
		},
		{
			"Member requester looking up themself",
			"k8s",
			"req",
			true,
			true,
			false,
		},
		{
			"Member requester looking up member",
			"k8s",
			"mem",
			true,
			true,
			false,
		},
		{
			"Member requester looking up non-member",
			"k8s",
			"nonmem",
			true,
			false,
			false,
		},
	}
	c := &Client{
		client: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		},
		base: ts.URL,
	}
	for _, tc := range testcases {
		if tc.reqMember {
			c.token = "member"
		} else {
			c.token = "nonmember"
		}
		member, err := c.IsMember(tc.org, tc.user)
		if (err != nil) != tc.expectedErr {
			t.Errorf("Wrong error response for case \"%s\". Got %v, expected %v", tc.description, err, tc.expectedErr)
		} else if member != tc.expectedMember {
			t.Errorf("Wrong membership for case \"%s\". Got %v, expected %v", tc.description, member, tc.expectedMember)
		}
	}
}
