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

package html

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/spyglass/api"
)

type FakeArtifact struct {
	path      string
	content   []byte
	sizeLimit int64
}

func (fa *FakeArtifact) JobPath() string {
	return fa.path
}

func (fa *FakeArtifact) Size() (int64, error) {
	return int64(len(fa.content)), nil
}

func (fa *FakeArtifact) CanonicalLink() string {
	return fa.path
}

func (fa *FakeArtifact) ReadAt(b []byte, off int64) (int, error) {
	r := bytes.NewReader(fa.content)
	return r.ReadAt(b, off)
}

func (fa *FakeArtifact) ReadAll() ([]byte, error) {
	return fa.content, nil
}

func (fa *FakeArtifact) ReadTail(n int64) ([]byte, error) {
	return nil, nil
}

func (fa *FakeArtifact) UseContext(ctx context.Context) error {
	return nil
}

func (fa *FakeArtifact) ReadAtMost(n int64) ([]byte, error) {
	return nil, nil
}

func TestRenderBody(t *testing.T) {
	testCases := []struct {
		name     string
		artifact FakeArtifact
	}{
		{
			name: "Simple",
			artifact: FakeArtifact{
				path:    "https://s3.internal/bucket/file.html",
				content: []byte(`<body>Hello world!</body>`),
			},
		},
		{
			name: "With quotes",
			artifact: FakeArtifact{
				path:    "https://s3.internal/bucket/file.html",
				content: []byte(`<body>"Hello world!"</body>`),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			body := (Lens{}).Body([]api.Artifact{&tc.artifact}, ".", "", nil, config.Spyglass{})
			fixtureName := filepath.Join("testdata", fmt.Sprintf("%s.yaml", strings.ReplaceAll(t.Name(), "/", "_")))
			if os.Getenv("UPDATE") != "" {
				if err := ioutil.WriteFile(fixtureName, []byte(body), 0644); err != nil {
					t.Errorf("failed to update fixture: %v", err)
				}
			}
			expectedRaw, err := ioutil.ReadFile(fixtureName)
			if err != nil {
				t.Fatalf("failed to read fixture: %v", err)
			}
			expected := string(expectedRaw)
			if diff := cmp.Diff(expected, body); diff != "" {
				t.Errorf("expected doesn't match actual: \n%s\nIf this is expected, re-run the tests with the UPDATE env var set to update the fixture:\n\tUPDATE=true go test ./prow/spyglass/lenses/html/... -run TestRenderBody", diff)
			}
		})
	}
}
