/*
Copyright 2018 The Kubernetes Authors.

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

package lenses

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"testing"

	"github.com/sirupsen/logrus"

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
	return "linknotfound.io/404"
}

func (fa *FakeArtifact) ReadAt(b []byte, off int64) (int, error) {
	r := bytes.NewReader(fa.content)
	return r.ReadAt(b, off)
}

func (fa *FakeArtifact) ReadAll() ([]byte, error) {
	size, err := fa.Size()
	if err != nil {
		return nil, err
	}
	if size > fa.sizeLimit {
		return nil, ErrFileTooLarge
	}
	r := bytes.NewReader(fa.content)
	return ioutil.ReadAll(r)
}

func (fa *FakeArtifact) ReadTail(n int64) ([]byte, error) {
	size, err := fa.Size()
	if err != nil {
		return nil, err
	}
	buf := make([]byte, n)
	_, err = fa.ReadAt(buf, size-n)
	return buf, err
}

func (fa *FakeArtifact) UseContext(ctx context.Context) error {
	return nil
}

func (fa *FakeArtifact) ReadAtMost(n int64) ([]byte, error) {
	buf := make([]byte, n)
	_, err := fa.ReadAt(buf, 0)
	return buf, err
}

type dumpLens struct{}

func (dumpLens) Config() LensConfig {
	return LensConfig{
		Name:  "dump",
		Title: "Dump Lens",
	}
}

func (dumpLens) Header(artifacts []api.Artifact, resourceDir string, config json.RawMessage, spyglassConfig config.Spyglass) string {
	return ""
}

func (dumpLens) Body(artifacts []api.Artifact, resourceDir string, data string, config json.RawMessage, spyglassConfig config.Spyglass) string {
	var view []byte
	for _, a := range artifacts {
		data, err := a.ReadAll()
		if err != nil {
			logrus.WithError(err).Error("Error reading artifact")
			continue
		}
		view = append(view, data...)
	}
	return string(view)
}

func (dumpLens) Callback(artifacts []api.Artifact, resourceDir string, data string, config json.RawMessage, spyglassConfig config.Spyglass) string {
	return ""
}

// Tests getting a view from a viewer
func TestView(t *testing.T) {
	err := RegisterLens(dumpLens{})
	if err != nil {
		t.Fatal("Failed to register viewer for testing View")
	}
	fakeLog := &FakeArtifact{
		path:      "log.txt",
		content:   []byte("Oh wow\nlogs\nthis is\ncrazy"),
		sizeLimit: 500e6,
	}
	testCases := []struct {
		name      string
		lensName  string
		artifacts []api.Artifact
		raw       string
		expected  string
		err       error
	}{
		{
			name:     "simple view",
			lensName: "dump",
			artifacts: []api.Artifact{
				fakeLog, fakeLog,
			},
			raw: "",
			expected: `Oh wow
logs
this is
crazyOh wow
logs
this is
crazy`,
			err: nil,
		},
		{
			name:      "fail on unregistered view name",
			lensName:  "MicroverseBattery",
			artifacts: []api.Artifact{},
			raw:       "",
			expected:  "",
			err:       ErrInvalidLensName,
		},
	}
	for _, tc := range testCases {
		lens, err := GetLens(tc.lensName)
		if tc.err != err {
			t.Errorf("%s expected error %v but got error %v", tc.name, tc.err, err)
			continue
		}
		if tc.err == nil && lens == nil {
			t.Fatalf("Expected lens %s but got nil.", tc.lensName)
		}
		if lens != nil && lens.Body(tc.artifacts, "", tc.raw, nil, config.Spyglass{}) != tc.expected {
			t.Errorf("%s expected view to be %s but got %s", tc.name, tc.expected, lens)
		}
	}
	UnregisterLens("DumpView")

}

// Tests reading last N Lines from files in GCS
func TestLastNLines_GCS(t *testing.T) {
	fakeGCSServerChunkSize := int64(3500)
	var longLog string
	for i := 0; i < 300; i++ {
		longLog += "here a log\nthere a log\neverywhere a log log\n"
	}
	testCases := []struct {
		name     string
		path     string
		contents []byte
		n        int64
		a        api.Artifact
		expected []string
	}{
		{
			name:     "Read last 2 lines of a 4-line file",
			n:        2,
			path:     "log.txt",
			contents: []byte("Oh wow\nlogs\nthis is\ncrazy"),
			expected: []string{"this is", "crazy"},
		},
		{
			name:     "Read last 5 lines of a 4-line file",
			n:        5,
			path:     "log.txt",
			contents: []byte("Oh wow\nlogs\nthis is\ncrazy"),
			expected: []string{"Oh wow", "logs", "this is", "crazy"},
		},
		{
			name:     "Read last 2 lines of a long log file",
			n:        2,
			path:     "long-log.txt",
			contents: []byte(longLog),
			expected: []string{
				"there a log",
				"everywhere a log log",
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			artifact := &FakeArtifact{
				path:      tc.path,
				content:   tc.contents,
				sizeLimit: 500e6,
			}
			actual, err := LastNLinesChunked(artifact, tc.n, fakeGCSServerChunkSize)
			if err != nil {
				t.Fatalf("failed with error: %v", err)
			}
			if len(actual) != len(tc.expected) {
				t.Fatalf("Expected length:\n%d\nActual length:\n%d", len(tc.expected), len(actual))
			}
			for ix, line := range tc.expected {
				if line != actual[ix] {
					t.Errorf("Line %d expected:\n%s\nActual line %d:\n%s", ix, line, ix, actual[ix])
					break
				}
			}
			for ix, line := range actual {
				if line != tc.expected[ix] {
					t.Errorf("Line %d expected:\n%s\nActual line %d:\n%s", ix, tc.expected[ix], ix, line)
					break
				}
			}
		})
	}
}
