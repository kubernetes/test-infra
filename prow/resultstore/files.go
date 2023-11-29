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
	"io"
	"strings"

	"google.golang.org/genproto/googleapis/devtools/resultstore/v2"
	"google.golang.org/protobuf/types/known/wrapperspb"
	pio "k8s.io/test-infra/prow/io"
)

// fileFinder is the subset of pio.Opener required.
type fileFinder interface {
	Iterator(ctx context.Context, prefix, delimiter string) (pio.ObjectIterator, error)
	Attributes(ctx context.Context, name string) (pio.Attributes, error)
}

// ArtifactFiles returns the files in directory dir, then all files
// in the artifacts/ subtree.
//
// In the event of error, returns any files collected so far in the
// interest of best effort.
func ArtifactFiles(ctx context.Context, opener fileFinder, dir string) ([]*resultstore.File, error) {
	prefix := ensureTrailingSlash(dir)
	c := newFilesCollector(opener, prefix)

	// Collect the files in the top-level dir.
	if err := c.collect(ctx, prefix, "/"); err != nil {
		return c.builder.Files(), err
	}

	// Collect the entire artifacts/ subtree.
	if err := c.collect(ctx, prefix+"artifacts/", ""); err != nil {
		return c.builder.Files(), err
	}
	return c.builder.Files(), nil
}

func ensureTrailingSlash(p string) string {
	if strings.HasSuffix(p, "/") {
		return p
	}
	return p + "/"
}

type filesCollector struct {
	finder  fileFinder
	builder *filesBuilder
}

func newFilesCollector(opener fileFinder, prefix string) *filesCollector {
	return &filesCollector{
		finder:  opener,
		builder: newFilesBuilder(prefix),
	}
}

// collect collects files from storage using GCS List semantics:
// - prefix should be a "/" terminated path.
// - delimiter should be "/" to search a single dir
// - delimiter should be "" to search the tree below prefix
func (c *filesCollector) collect(ctx context.Context, prefix, delimiter string) error {
	iter, err := c.finder.Iterator(ctx, prefix, delimiter)
	if err != nil {
		return err
	}
	for {
		f, err := iter.Next(ctx)
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		if f.IsDir {
			continue
		}
		attrs, err := c.finder.Attributes(ctx, f.Name)
		if err != nil {
			return err
		}
		c.builder.Add(f.Name, &attrs)
	}
	return nil
}

type filesBuilder struct {
	prefix string
	trim   func(string) string
	files  []*resultstore.File
}

func newFilesBuilder(prefix string) *filesBuilder {
	return &filesBuilder{
		prefix: prefix,
		// Trims the prefix from names to produce File.Uid.
		trim: strings.NewReplacer(prefix, "").Replace,
	}
}

func (b *filesBuilder) Add(name string, attrs *pio.Attributes) {
	uid := b.trim(name)
	switch uid {
	case "build.log":
		// This file name is unexpected and would cause an upload
		// exception, since ResultStore requires unique Uids.
		return
	case "build-log.txt":
		// This Uid is used to populate the "Build Log" tab in the
		// GUI. We want build-log.txt to appear there.
		uid = "build.log"
	}
	b.files = append(b.files, &resultstore.File{
		Uid:         uid,
		Uri:         name,
		Length:      &wrapperspb.Int64Value{Value: attrs.Size},
		ContentType: shortContentType(attrs.ContentEncoding),
	})
}

func shortContentType(contentType string) string {
	ps := strings.SplitN(contentType, ";", 2)
	return ps[0]
}

func (b *filesBuilder) Files() []*resultstore.File {
	return b.files
}
