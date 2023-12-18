/*
Copyright 2023 The Kubernetes Authors.

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

package resultstore

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/genproto/googleapis/devtools/resultstore/v2"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"k8s.io/apimachinery/pkg/util/sets"
	pio "k8s.io/test-infra/prow/io"
)

// fakeFileFinder is a testing fake for a subset of pio.Opener.
type fakeFileFinder struct {
	files map[string]pio.Attributes
}

// Assert this matches our subset of pio.Opener.
var _ fileFinder = &fakeFileFinder{}

var (
	// A file with this value causes Iterator() to return error.
	wantIterErr = pio.Attributes{ContentEncoding: "__wantIterErr__"}
	// A file with this value causes Iterator.Next() to return error.
	wantNextErr = pio.Attributes{ContentEncoding: "__wantNextErr__"}
)

// Iterator iterates at a prefix, which includes the provider and
// bucket; the output names are relative to the bucket.
func (f *fakeFileFinder) Iterator(_ context.Context, prefix, delimiter string) (pio.ObjectIterator, error) {
	var fs []string
	seenDirs := sets.Set[string]{}
	b, err := bucket(prefix)
	if err != nil {
		return nil, err
	}
	for n, a := range f.files {
		if !strings.HasPrefix(n, prefix) {
			continue
		}
		if delimiter == "" || !strings.Contains(n[len(prefix):], delimiter) {
			if a.ContentEncoding == wantIterErr.ContentEncoding {
				return nil, fmt.Errorf("iterator error at %q", n)
			}
			fs = append(fs, n)
			continue
		}
		ps := strings.SplitN(n[len(prefix):], delimiter, 2)
		if seenDirs.Has(ps[0]) {
			continue
		}
		fs = append(fs, fmt.Sprintf("%s%s/", prefix, ps[0]))
		seenDirs.Insert(ps[0])
	}
	slices.Sort(fs)
	return &fakeIterator{finder: f, files: fs, bucket: b}, nil
}

type fakeIterator struct {
	finder *fakeFileFinder
	files  []string
	bucket string
	pos    int
}

// Next only populates ObjectAttributes fields used by this package:
// Name and IsDir.
func (i *fakeIterator) Next(_ context.Context) (pio.ObjectAttributes, error) {
	oa := pio.ObjectAttributes{}
	if i.pos >= len(i.files) {
		return oa, io.EOF
	}
	n := i.files[i.pos]
	i.pos++
	if i.finder.files[n].ContentEncoding == wantNextErr.ContentEncoding {
		return oa, fmt.Errorf("next error at %q", n)
	}
	oa.Name = strings.TrimPrefix(n, i.bucket)
	oa.IsDir = strings.HasSuffix(n, "/")
	oa.Size = i.finder.files[n].Size
	return oa, nil
}

func TestArtifactFiles(t *testing.T) {
	ctx := context.Background()
	base := "gs://bucket/pr-logs/1234"
	for _, tc := range []struct {
		desc    string
		ff      *fakeFileFinder
		opts    ArtifactOpts
		want    []*resultstore.File
		wantErr bool
	}{
		{
			desc: "success",
			ff: &fakeFileFinder{
				files: map[string]pio.Attributes{
					base + "/build-log.txt": {
						Size: 9000,
					},
					base + "/started.json": {
						Size: 350,
					},
					base + "/artifacts/artifact.txt": {
						Size: 10000,
					},
				},
			},
			opts: ArtifactOpts{
				Dir: base,
				DefaultFiles: []DefaultFile{
					{
						Name: "prowjob.json",
						Size: 1984,
					},
					{
						Name: "started.json",
						Size: 3500,
					},
				},
			},
			want: []*resultstore.File{
				{
					Uid:         "build.log",
					Uri:         "gs://bucket/pr-logs/1234/build-log.txt",
					Length:      &wrapperspb.Int64Value{Value: 9000},
					ContentType: "text/plain",
				},
				{
					Uid:         "started.json",
					Uri:         "gs://bucket/pr-logs/1234/started.json",
					Length:      &wrapperspb.Int64Value{Value: 350},
					ContentType: "application/json",
				},
				{
					Uid:         "prowjob.json",
					Uri:         "gs://bucket/pr-logs/1234/prowjob.json",
					Length:      &wrapperspb.Int64Value{Value: 1984},
					ContentType: "application/json",
				},
				{
					Uid:         "artifacts/artifact.txt",
					Uri:         "gs://bucket/pr-logs/1234/artifacts/artifact.txt",
					Length:      &wrapperspb.Int64Value{Value: 10000},
					ContentType: "text/plain",
				},
			},
		},
		{
			desc: "artifacts dir",
			ff: &fakeFileFinder{
				files: map[string]pio.Attributes{
					base + "/build-log.txt": {
						Size: 9000,
					},
					base + "/started.json": {
						Size: 350,
					},
					base + "/artifacts/artifact.txt": {
						Size: 10000,
					},
				},
			},
			opts: ArtifactOpts{
				Dir:              base,
				ArtifactsDirOnly: true,
			},
			want: []*resultstore.File{
				{
					Uid:         "build.log",
					Uri:         "gs://bucket/pr-logs/1234/build-log.txt",
					Length:      &wrapperspb.Int64Value{Value: 9000},
					ContentType: "text/plain",
				},
				{
					Uid:         "started.json",
					Uri:         "gs://bucket/pr-logs/1234/started.json",
					Length:      &wrapperspb.Int64Value{Value: 350},
					ContentType: "application/json",
				},
				{
					Uid: "artifacts/",
					Uri: "gs://bucket/pr-logs/1234/artifacts/",
				},
			},
		},
		{
			desc: "exclude unwanted subdirs",
			ff: &fakeFileFinder{
				files: map[string]pio.Attributes{
					base + "/not-artifacts-subdir/unwanted": {},
				},
			},
			opts: ArtifactOpts{
				Dir: base,
			},
			want: nil,
		},
		{
			desc: "exclude build.log",
			ff: &fakeFileFinder{
				files: map[string]pio.Attributes{
					base + "/build.log": {},
				},
			},
			opts: ArtifactOpts{
				Dir: base,
			},
			want: nil,
		},
		{
			desc: "empty",
			ff:   &fakeFileFinder{},
			opts: ArtifactOpts{
				Dir: base,
			},
			want: nil,
		},
		{
			desc: "iterator error",
			ff: &fakeFileFinder{
				files: map[string]pio.Attributes{
					base + "/iterator-error.txt": wantIterErr,
				},
			},
			opts: ArtifactOpts{
				Dir: base,
			},
			wantErr: true,
		},
		{
			desc: "iteration error",
			ff: &fakeFileFinder{
				files: map[string]pio.Attributes{
					base + "/expected.txt": {
						Size: 100,
					},
					base + "/next-error.txt": wantNextErr,
					base + "/artifacts/unexpected.txt": {
						Size: 1000,
					},
				},
			},
			opts: ArtifactOpts{
				Dir: base,
			},
			want: []*resultstore.File{
				{
					Uid:         "expected.txt",
					Uri:         "gs://bucket/pr-logs/1234/expected.txt",
					Length:      &wrapperspb.Int64Value{Value: 100},
					ContentType: "text/plain",
				},
			},
			wantErr: true,
		},
		{
			desc: "artifacts iteration error",
			ff: &fakeFileFinder{
				files: map[string]pio.Attributes{
					base + "/ok.txt": {
						Size: 100,
					},
					base + "/artifacts/a.txt": {
						Size: 1000,
					},
					base + "/artifacts/b.txt": wantNextErr,
					base + "/artifacts/c.txt": {
						Size: 10000,
					},
				},
			},
			opts: ArtifactOpts{
				Dir: base,
			},
			want: []*resultstore.File{
				{
					Uid:         "ok.txt",
					Uri:         "gs://bucket/pr-logs/1234/ok.txt",
					Length:      &wrapperspb.Int64Value{Value: 100},
					ContentType: "text/plain",
				},
				{
					Uid:         "artifacts/a.txt",
					Uri:         "gs://bucket/pr-logs/1234/artifacts/a.txt",
					Length:      &wrapperspb.Int64Value{Value: 1000},
					ContentType: "text/plain",
				},
			},
			wantErr: true,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			got, err := ArtifactFiles(ctx, tc.ff, tc.opts)
			if err != nil {
				if tc.wantErr {
					t.Logf("got expected error: %v", err)
				} else {
					t.Fatalf("got unwanted error: %v", err)
				}
			} else if tc.wantErr {
				t.Fatal("wanted error, got nil")
			}
			if diff := cmp.Diff(tc.want, got, protocmp.Transform()); diff != "" {
				t.Errorf("diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestEnsureTrailingSlash(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
	}{
		{"some/path", "some/path/"},
		{"some/path/", "some/path/"},
	} {
		if got := ensureTrailingSlash(tc.in); got != tc.want {
			t.Errorf("ensureTrailingSlash(%q) got %s, want %s", tc.in, got, tc.want)
		}
	}
}
