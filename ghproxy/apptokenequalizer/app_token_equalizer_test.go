/*
Copyright 2020 The Kubernetes Authors.

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

package apptokenequalizer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"k8s.io/test-infra/prow/github"
)

func TestRoundTrip(t *testing.T) {
	const appID = "appid"
	oneHourInFuture := time.Now().Add(time.Hour)
	testCases := []struct {
		name             string
		tokenCache       map[string]github.AppInstallationToken
		requestPath      string
		delegateResponse *http.Response
		delegateError    error
		expectedResponse *http.Response
		expectedError    error
	}{
		{
			name:        "Response is served from cache",
			tokenCache:  map[string]github.AppInstallationToken{appID: {Token: "token", ExpiresAt: oneHourInFuture}},
			requestPath: fmt.Sprintf("/app/installations/%s/tokens", appID),
			delegateResponse: &http.Response{
				StatusCode: 201,
				Header:     http.Header{},
				Body:       serializeOrDie(github.AppInstallationToken{Token: "other-token", ExpiresAt: time.Now().Add(time.Hour)}),
			},
			expectedResponse: &http.Response{
				StatusCode: 201,
				Body:       serializeOrDie(github.AppInstallationToken{Token: "token", ExpiresAt: oneHourInFuture}),
			},
		},
		{
			name:          "Delegate error is passed on and response is not from cache",
			tokenCache:    map[string]github.AppInstallationToken{appID: {Token: "token", ExpiresAt: oneHourInFuture}},
			requestPath:   fmt.Sprintf("/app/installations/%s/tokens", appID),
			delegateError: ComparableError("some-error"),
			expectedError: ComparableError("some-error"),
		},
		{
			name:        "Status is not 201, response is not from cache",
			tokenCache:  map[string]github.AppInstallationToken{appID: {Token: "token", ExpiresAt: oneHourInFuture}},
			requestPath: fmt.Sprintf("/app/installations/%s/tokens", appID),
			delegateResponse: &http.Response{
				StatusCode: 200,
				Header:     http.Header{},
				Body:       serializeOrDie(github.AppInstallationToken{Token: "other-token", ExpiresAt: oneHourInFuture}),
			},
			expectedResponse: &http.Response{
				StatusCode: 200,
				Body:       serializeOrDie(github.AppInstallationToken{Token: "other-token", ExpiresAt: oneHourInFuture}),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			transport := &appTokenEqualizerTransport{
				tokenCache: tc.tokenCache,
				delegate: &fakeRoundTripper{
					response: tc.delegateResponse,
					err:      tc.delegateError,
				},
			}

			r, err := http.NewRequest(http.MethodPost, tc.requestPath, nil)
			if err != nil {
				t.Fatalf("failed to construct request: %v", err)
			}

			response, err := transport.RoundTrip(r)
			if diff := cmp.Diff(err, tc.expectedError); diff != "" {
				t.Fatalf("actual error differs from expected: %s", diff)
			}
			if err != nil {
				return
			}

			if diff := cmp.Diff(response.StatusCode, tc.expectedResponse.StatusCode); diff != "" {
				t.Errorf("actual status code differs from expected: %s", diff)
			}

			actualBody, err := ioutil.ReadAll(response.Body)
			if err != nil {
				t.Fatalf("failed to read actual response body: %v", err)
			}
			expectedBody, err := ioutil.ReadAll(tc.expectedResponse.Body)
			if err != nil {
				t.Fatalf("failed to read expected response body: %v", err)
			}
			if diff := cmp.Diff(string(actualBody), string(expectedBody)); diff != "" {
				t.Errorf("actual response differs from expectedResponse: %s", diff)
			}
		})
	}
}

func serializeOrDie(in interface{}) io.ReadCloser {
	rawData, err := json.Marshal(in)
	if err != nil {
		panic(fmt.Sprintf("Serialization failed: %v", err))
	}
	return ioutil.NopCloser(bytes.NewBuffer(rawData))
}

type fakeRoundTripper struct {
	response *http.Response
	err      error
}

func (frt *fakeRoundTripper) RoundTrip(_ *http.Request) (*http.Response, error) {
	return frt.response, frt.err
}

type ComparableError string

func (c ComparableError) Error() string {
	return string(c)
}
