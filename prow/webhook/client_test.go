/*
Copyright 2021 The Kubernetes Authors.

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

package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSend(t *testing.T) {
	cancelledContext, cancel := context.WithCancel(context.Background())
	cancel()

	expectNil := func(err error) error {
		if err != nil {
			return fmt.Errorf("unexpected error: %v", err)
		}
		return nil
	}
	expectErrorContainingMessage := func(expected string) func(error) error {
		return func(actual error) error {
			if !strings.Contains(actual.Error(), expected) {
				return fmt.Errorf("expected error %q to contain %q", actual, expected)
			}
			return nil
		}
	}

	for _, tc := range [...]struct {
		name    string
		context context.Context
		token   string
		body    string

		expectedError func(error) error
		expectedCall  bool
	}{
		{
			name:    "happy path",
			context: context.Background(),
			token:   "the token",
			body:    "the message",

			expectedCall:  true,
			expectedError: expectNil,
		},
		{
			name:    "canceled",
			context: cancelledContext,

			expectedCall:  false,
			expectedError: expectErrorContainingMessage(context.Canceled.Error()),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var called bool
			server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				called = true

				if expected, actual := tc.token, req.Header.Get("x-prow-token"); expected != actual {
					t.Errorf("expected token %q, found %q", expected, actual)
				}

				if timestamp := req.Header.Get("x-prow-timestamp"); timestamp != "" {
					if _, err := time.Parse(time.RFC3339, timestamp); err != nil {
						t.Errorf("unexpected error while parsing the timestamp as RFC3339: %v", err)
					}
				} else {
					t.Errorf("expected x-prow-timestamp header, found empty or unset")
				}

				var actualBody string
				if err := json.NewDecoder(req.Body).Decode(&actualBody); err != nil {
					t.Fatalf("unexpected error while decoding the request body: %v", err)
				}

				if expected, actual := tc.body, actualBody; expected != actual {
					t.Errorf("expected webhook body %q, found %q", expected, actual)
				}
			}))
			defer server.Close()

			client := NewClient(func(_ []byte) (string, error) { return tc.token, nil })

			if err := tc.expectedError(client.Send(tc.context, server.URL, tc.body)); err != nil {
				t.Error(err)
			}

			if tc.expectedCall != called {
				if tc.expectedCall {
					t.Errorf("expected call to the webhook URL, not received")
				} else {
					t.Errorf("unexpected call to the webhook URL")
				}
			}
		})
	}
}

func TestDigest(t *testing.T) {
	merge := func(slices ...[]byte) []byte {
		var b []byte
		for _, slice := range slices {
			b = append(b, slice...)
		}
		return b
	}

	for _, tc := range [...]struct {
		name      string
		url       string
		timestamp string
		message   []byte
		expected  []byte
	}{
		{
			name:     "zero values",
			expected: []byte{0, 0},
		},
		{
			name:      "example message",
			url:       "https://webhook.example.com:1234/hello#world",
			timestamp: "2023-01-03T14:25:15Z",
			message:   []byte("Hello world"),
			expected: merge(
				[]byte("https://webhook.example.com:1234/hello#world"),
				[]byte{0},
				[]byte("2023-01-03T14:25:15Z"),
				[]byte{0},
				[]byte("Hello world"),
			),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if computed := digest(tc.url, tc.timestamp, tc.message); !bytes.Equal(computed, tc.expected) {
				t.Errorf("mismatch: expected %v, got %v", tc.expected, computed)
			}
		})
	}
}
