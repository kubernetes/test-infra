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
package helloworld

import (
	"context"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"testing"

	"cloud.google.com/go/functions/metadata"
)

func TestHelloGCS(t *testing.T) {
	type testCase struct {
		name        string
		event       GCSEvent
		expectedErr bool
	}
	testcases := []testCase{
		{
			name: "non-finished artifact",
			event: GCSEvent{
				Name: "logs/kubeflow-examples-periodic/1346565584548007936/artifacts/junit_mnist-notebook/mnist_test/20200808-1530-70d4/notebook.html",
			},
			expectedErr: true,
		},
		{
			name: "finished artifact",
			event: GCSEvent{
				Name: "logs/kubeflow-examples-periodic/1346565584548007936/artifacts/junit_mnist-notebook/mnist_test/20200808-1530-70d4/finished.json",
			},
			expectedErr: true,
		},
	}
	for _, tt := range testcases {
		r, w, _ := os.Pipe()
		log.SetOutput(w)
		originalFlags := log.Flags()
		log.SetFlags(log.Flags() &^ (log.Ldate | log.Ltime))

		meta := &metadata.Metadata{
			EventID: "event ID",
		}
		ctx := metadata.NewContext(context.Background(), meta)

		err := HelloGCS(ctx, tt.event)
		if err != nil && tt.expectedErr {
			if tt.expectedErr {
				continue
			}
			t.Fatalf("expected error for test: %v", tt)
		}

		w.Close()
		log.SetOutput(os.Stderr)
		log.SetFlags(originalFlags)

		out, err := ioutil.ReadAll(r)
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}

		got := string(out)
		wants := []string{
			"File: " + tt.event.Name,
			"Event ID: " + meta.EventID,
		}
		for _, want := range wants {
			if !strings.Contains(got, want) {
				t.Errorf("HelloGCS(%v) = %q, want to contain %q", tt.event, got, want)
			}
		}
	}
}
