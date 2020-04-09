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

package history

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"testing"
	"time"

	"cloud.google.com/go/storage"
	"k8s.io/apimachinery/pkg/util/diff"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	pkgio "k8s.io/test-infra/prow/io"
)

func TestHistory(t *testing.T) {
	var nowTime = time.Now()
	oldNow := now
	now = func() time.Time { return nowTime }
	defer func() { now = oldNow }()

	const logSizeLimit = 3
	nextTime := func() time.Time {
		nowTime = nowTime.Add(time.Minute)
		return nowTime
	}

	testMeta := func(num int, author string) prowapi.Pull {
		return prowapi.Pull{
			Number: num,
			Title:  fmt.Sprintf("PR #%d", num),
			SHA:    fmt.Sprintf("SHA for %d", num),
			Author: author,
		}
	}

	hist, err := New(logSizeLimit, nil, "")
	if err != nil {
		t.Fatalf("Failed to create history client: %v", err)
	}
	time1 := nextTime()
	hist.Record("pool A", "TRIGGER", "sha A", "", []prowapi.Pull{testMeta(1, "bob")})
	nextTime()
	hist.Record("pool B", "MERGE", "sha B1", "", []prowapi.Pull{testMeta(2, "joe")})
	time3 := nextTime()
	hist.Record("pool B", "MERGE", "sha B2", "", []prowapi.Pull{testMeta(3, "jeff")})
	time4 := nextTime()
	hist.Record("pool B", "MERGE_BATCH", "sha B3", "", []prowapi.Pull{testMeta(4, "joe"), testMeta(5, "jim")})
	time5 := nextTime()
	hist.Record("pool C", "TRIGGER_BATCH", "sha C1", "", []prowapi.Pull{testMeta(6, "joe"), testMeta(8, "me")})
	time6 := nextTime()
	hist.Record("pool B", "TRIGGER", "sha B4", "", []prowapi.Pull{testMeta(7, "abe")})

	expected := map[string][]*Record{
		"pool A": {
			&Record{
				Time:    time1,
				BaseSHA: "sha A",
				Action:  "TRIGGER",
				Target: []prowapi.Pull{
					testMeta(1, "bob"),
				},
			},
		},
		"pool B": {
			&Record{
				Time:    time6,
				BaseSHA: "sha B4",
				Action:  "TRIGGER",
				Target: []prowapi.Pull{
					testMeta(7, "abe"),
				},
			},
			&Record{
				Time:    time4,
				BaseSHA: "sha B3",
				Action:  "MERGE_BATCH",
				Target: []prowapi.Pull{
					testMeta(4, "joe"),
					testMeta(5, "jim"),
				},
			},
			&Record{
				Time:    time3,
				BaseSHA: "sha B2",
				Action:  "MERGE",
				Target: []prowapi.Pull{
					testMeta(3, "jeff"),
				},
			},
		},
		"pool C": {
			&Record{
				Time:    time5,
				BaseSHA: "sha C1",
				Action:  "TRIGGER_BATCH",
				Target: []prowapi.Pull{
					testMeta(6, "joe"),
					testMeta(8, "me"),
				},
			},
		},
	}

	if got := hist.AllRecords(); !reflect.DeepEqual(got, expected) {
		es, _ := json.Marshal(expected)
		gs, _ := json.Marshal(got)
		t.Errorf("Expected history \n%s, but got \n%s.", es, gs)
		t.Logf("strs equal: %v.", string(es) == string(gs))
	}
}

const fakePath = "/some/random/path"

type testOpener struct {
	content string
	closed  bool
	dne     bool
}

func (t *testOpener) Reader(ctx context.Context, path string) (io.ReadCloser, error) {
	if t.dne {
		return nil, storage.ErrObjectNotExist
	}
	if path != fakePath {
		return nil, fmt.Errorf("path %q != expected %q", path, fakePath)
	}
	return t, nil
}

func (t *testOpener) Writer(ctx context.Context, path string, _ ...pkgio.WriterOptions) (io.WriteCloser, error) {
	if path != fakePath {
		return nil, fmt.Errorf("path %q != expected %q", path, fakePath)
	}
	return t, nil
}

func (t *testOpener) Write(p []byte) (n int, err error) {
	if t.closed {
		return 0, errors.New("writer is already closed")
	}
	t.content += string(p)
	return len(p), nil
}

func (t *testOpener) Read(p []byte) (n int, err error) {
	if t.closed {
		return 0, errors.New("reader is already closed")
	}
	if len(t.content) == 0 {
		return 0, io.EOF
	}
	defer func() { t.content = t.content[n:] }()
	return copy(p, t.content), nil
}

func (t *testOpener) Close() error {
	if t.closed {
		return errors.New("already closed")
	}
	t.closed = true
	return nil
}

func TestReadHistory(t *testing.T) {
	tcs := []struct {
		name           string
		raw            string
		maxRecsPerPool int
		dne            bool
		expectedHist   map[string]*recordLog
	}{
		{
			name:           "read empty history",
			raw:            `{}`,
			maxRecsPerPool: 3,
			expectedHist:   map[string]*recordLog{},
		},
		{
			name:           "read non-existent history",
			dne:            true,
			maxRecsPerPool: 3,
			expectedHist:   map[string]*recordLog{},
		},
		{
			name:           "read simple history",
			raw:            `{"o/r:b":[{"time":"0001-01-01T00:00:00Z","action":"MERGE"}]}`,
			maxRecsPerPool: 3,
			expectedHist: map[string]*recordLog{
				"o/r:b": {buff: []*Record{{Action: "MERGE"}}, head: 0, limit: 3},
			},
		},
		{
			name:           "read history with full recordLog",
			raw:            `{"o/r:b":[{"time":"0001-01-01T00:00:00Z","action":"MERGE4"},{"time":"0001-01-01T00:00:00Z","action":"MERGE3"},{"time":"0001-01-01T00:00:00Z","action":"MERGE2"}]}`,
			maxRecsPerPool: 3,
			expectedHist: map[string]*recordLog{
				"o/r:b": {buff: []*Record{{Action: "MERGE2"}, {Action: "MERGE3"}, {Action: "MERGE4"}}, head: 2, limit: 3},
			},
		},
		{
			name:           "read history, with multiple pools",
			raw:            `{"o/r:b":[{"time":"0001-01-01T00:00:00Z","action":"MERGE"}],"o/r:b2":[{"time":"0001-01-01T00:00:00Z","action":"MERGE2"},{"time":"0001-01-01T00:00:00Z","action":"MERGE"}]}`,
			maxRecsPerPool: 3,
			expectedHist: map[string]*recordLog{
				"o/r:b":  {buff: []*Record{{Action: "MERGE"}}, head: 0, limit: 3},
				"o/r:b2": {buff: []*Record{{Action: "MERGE"}, {Action: "MERGE2"}}, head: 1, limit: 3},
			},
		},
		{
			name:           "read and truncate",
			raw:            `{"o/r:b":[{"time":"0001-01-01T00:00:00Z","action":"MERGE3"},{"time":"0001-01-01T00:00:00Z","action":"MERGE2"},{"time":"0001-01-01T00:00:00Z","action":"MERGE1"}]}`,
			maxRecsPerPool: 2,
			expectedHist: map[string]*recordLog{
				"o/r:b": {buff: []*Record{{Action: "MERGE2"}, {Action: "MERGE3"}}, head: 1, limit: 2},
			},
		},
		{
			name:           "read and grow record log",
			raw:            `{"o/r:b":[{"time":"0001-01-01T00:00:00Z","action":"MERGE3"},{"time":"0001-01-01T00:00:00Z","action":"MERGE2"},{"time":"0001-01-01T00:00:00Z","action":"MERGE1"}]}`,
			maxRecsPerPool: 5,
			expectedHist: map[string]*recordLog{
				"o/r:b": {buff: []*Record{{Action: "MERGE1"}, {Action: "MERGE2"}, {Action: "MERGE3"}}, head: 2, limit: 5},
			},
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			obj := &testOpener{content: tc.raw, dne: tc.dne}
			hist, err := readHistory(tc.maxRecsPerPool, obj, fakePath)
			if err != nil {
				t.Fatalf("Unexpected error reading history: %v.", err)
			}
			if !reflect.DeepEqual(hist, tc.expectedHist) {
				t.Errorf("Unexpected diff between loaded history and expected history: %v.", diff.ObjectReflectDiff(hist, tc.expectedHist))
			}
			if !obj.closed && !tc.dne {
				t.Errorf("Reader was not closed.")
			}
		})
	}
}

func TestWriteHistory(t *testing.T) {
	tcs := []struct {
		name            string
		recMap          map[string][]*Record
		expectedWritten string
	}{
		{
			name:            "write empty history",
			recMap:          map[string][]*Record{},
			expectedWritten: `{}`,
		},
		{
			name: "write simple history",
			recMap: map[string][]*Record{
				"o/r:b": {{Action: "MERGE"}},
			},
			expectedWritten: `{"o/r:b":[{"time":"0001-01-01T00:00:00Z","action":"MERGE"}]}`,
		},
		{
			name: "write history with multiple records",
			recMap: map[string][]*Record{
				"o/r:b": {{Action: "MERGE3"}, {Action: "MERGE2"}, {Action: "MERGE1"}},
			},
			expectedWritten: `{"o/r:b":[{"time":"0001-01-01T00:00:00Z","action":"MERGE3"},{"time":"0001-01-01T00:00:00Z","action":"MERGE2"},{"time":"0001-01-01T00:00:00Z","action":"MERGE1"}]}`,
		},
		{
			name: "write history, with multiple pools",
			recMap: map[string][]*Record{
				"o/r:b":  {{Action: "MERGE"}},
				"o/r:b2": {{Action: "MERGE2"}, {Action: "MERGE1"}},
			},
			expectedWritten: `{"o/r:b":[{"time":"0001-01-01T00:00:00Z","action":"MERGE"}],"o/r:b2":[{"time":"0001-01-01T00:00:00Z","action":"MERGE2"},{"time":"0001-01-01T00:00:00Z","action":"MERGE1"}]}`,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			obj := &testOpener{}
			if err := writeHistory(obj, fakePath, tc.recMap); err != nil {
				t.Fatalf("Unexpected error writing history: %v.", err)
			}
			if obj.content != tc.expectedWritten {
				t.Errorf("Expected write:\n%s\nbut got:\n%s", tc.expectedWritten, obj.content)
			}
			if !obj.closed {
				t.Errorf("Writer was not closed.")
			}
		})
	}
}
