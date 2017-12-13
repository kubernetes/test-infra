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

package main

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestLinesByTimestampValidJSON(t *testing.T) {
	tests := []struct {
		name string
		log  linesByTimestamp
	}{
		{
			name: "no line",
			log:  linesByTimestamp{},
		},
		{
			name: "single line",
			log: linesByTimestamp{
				{
					actual: []byte("{\"component\":\"jenkins-operator\",\"event-GUID\":\"thisisunique\",\"from\":\"triggered\",\"job\":\"origin-ci-ut-origin\",\"level\":\"info\",\"msg\":\"Transitioning states.\",\"name\":\"50ea24ea-d9a9-11e7-8e52-0a58ac101211\",\"org\":\"openshift\",\"pr\":17586,\"repo\":\"origin\",\"time\":\"2017-12-05T10:45:14Z\",\"to\":\"pending\",\"type\":\"presubmit\"}\n"),
				},
			},
		},
		{
			name: "multiple lines",
			log: linesByTimestamp{
				{
					actual: []byte("{\"component\":\"jenkins-operator\",\"event-GUID\":\"thisisunique\",\"from\":\"triggered\",\"job\":\"origin-ci-ut-origin\",\"level\":\"info\",\"msg\":\"Transitioning states.\",\"name\":\"50ea24ea-d9a9-11e7-8e52-0a58ac101211\",\"org\":\"openshift\",\"pr\":17586,\"repo\":\"origin\",\"time\":\"2017-12-05T10:45:14Z\",\"to\":\"pending\",\"type\":\"presubmit\"}\n"),
				},
				{
					actual: []byte("{\"author\":\"bob\",\"event-GUID\":\"thisisunique\",\"event-type\":\"issue_comment\",\"job\":\"test_pull_request_origin_extended_conformance_install_update\",\"level\":\"info\",\"msg\":\"Creating a new prowjob.\",\"name\":\"50edc666-d9a9-11e7-8e52-0a58ac101211\",\"org\":\"openshift\",\"plugin\":\"trigger\",\"pr\":17586,\"repo\":\"origin\",\"time\":\"2017-12-05T10:44:48Z\",\"type\":\"presubmit\",\"url\":\"an_url\"}\n"),
				},
				{
					actual: []byte("{\"author\":\"bob\",\"event-GUID\":\"thisisunique\",\"event-type\":\"issue_comment\",\"level\":\"info\",\"msg\":\"Starting test_pull_request_origin_extended_networking_minimal build.\",\"org\":\"openshift\",\"plugin\":\"trigger\",\"pr\":17586,\"repo\":\"origin\",\"time\":\"2017-12-05T10:44:48Z\",\"url\":\"an_url\"}\n"),
				},
			},
		},
	}

	for _, test := range tests {
		jsonLog := test.log.String()
		if !json.Valid([]byte(jsonLog)) {
			t.Errorf("%s: got invalid json:\n%v", test.name, jsonLog)
		}
	}
}

func TestValidTraceRequest(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		expectedErr string
	}{
		{
			name:        "valid request - eventGUID",
			url:         "https://deck/trace?event-GUID=503265b0-d9a9-11e7-9b32-1fd823242322",
			expectedErr: "",
		},
		{
			name:        "valid request - org/repo#pr",
			url:         "https://deck/trace?repo=origin&org=openshift&pr=17586",
			expectedErr: "",
		},
		{
			name:        "valid request - org/repo#pr#issuecomment",
			url:         "https://deck/trace?repo=origin&org=openshift&pr=17586&issuecomment=350075289",
			expectedErr: "",
		},
		{
			name:        "invalid request - pr is not a number",
			url:         "https://deck/trace?repo=origin&org=openshift&pr=175fd",
			expectedErr: "invalid pr query \"175fd\": strconv.Atoi: parsing \"175fd\": invalid syntax",
		},
		{
			name:        "invalid request - pr is not a positive number",
			url:         "https://deck/trace?repo=origin&org=openshift&pr=-17453",
			expectedErr: "invalid pr query \"-17453\": needs to be a positive number",
		},
		{
			name:        "invalid request - missing org parameter",
			url:         "https://deck/trace?repo=origin&pr=17453",
			expectedErr: "need either \"pr\", \"repo\", and \"org\", or \"event-GUID\", or \"issuecomment\" to be specified",
		},
		{
			name:        "invalid request - missing repo parameter",
			url:         "https://deck/trace?org=openshift&pr=17453",
			expectedErr: "need either \"pr\", \"repo\", and \"org\", or \"event-GUID\", or \"issuecomment\" to be specified",
		},
		{
			name:        "invalid request - missing pr parameter",
			url:         "https://deck/trace?org=openshift&repo=origin",
			expectedErr: "need either \"pr\", \"repo\", and \"org\", or \"event-GUID\", or \"issuecomment\" to be specified",
		},
		{
			name:        "invalid request - missing org and repo parameter",
			url:         "https://deck/trace?pr=17453",
			expectedErr: "need either \"pr\", \"repo\", and \"org\", or \"event-GUID\", or \"issuecomment\" to be specified",
		},
		{
			name:        "invalid request - missing org and pr parameter",
			url:         "https://deck/trace?repo=origin",
			expectedErr: "need either \"pr\", \"repo\", and \"org\", or \"event-GUID\", or \"issuecomment\" to be specified",
		},
		{
			name:        "invalid request - missing repo and pr parameter",
			url:         "https://deck/trace?org=openshift",
			expectedErr: "need either \"pr\", \"repo\", and \"org\", or \"event-GUID\", or \"issuecomment\" to be specified",
		},
		{
			name:        "invalid request - no parameters",
			url:         "https://deck/trace",
			expectedErr: "need either \"pr\", \"repo\", and \"org\", or \"event-GUID\", or \"issuecomment\" to be specified",
		},
		{
			name:        "invalid request - issuecomment and event-GUID are mutually exclusive",
			url:         "https://deck/trace?repo=origin&org=openshift&pr=175&issuecomment=350075289&event-GUID=503265b0",
			expectedErr: "cannot specify both issuecomment (350075289) and event-GUID (503265b0)",
		},
	}

	for _, test := range tests {
		req, err := http.NewRequest(http.MethodGet, test.url, nil)
		if err != nil {
			t.Errorf("%s: unexpected error: %v", test.name, err)
			continue
		}
		gotErr := validateTraceRequest(req)
		if gotErr != nil && gotErr.Error() != test.expectedErr {
			t.Errorf("%s: unexpected error: %q, expected: %q", test.name, gotErr.Error(), test.expectedErr)
			continue
		}
		if test.expectedErr != "" && gotErr == nil {
			t.Errorf("%s: expected an error (%s) but got none", test.name, test.expectedErr)
		}
	}
}
