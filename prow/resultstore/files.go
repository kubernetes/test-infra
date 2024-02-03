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
	"mime"
	"path/filepath"
	"strings"

	"google.golang.org/genproto/googleapis/devtools/resultstore/v2"
	"google.golang.org/protobuf/types/known/wrapperspb"
	pio "k8s.io/test-infra/prow/io"
	"k8s.io/test-infra/prow/io/providers"
)

// fileFinder is the subset of pio.Opener required.
type fileFinder interface {
	Iterator(ctx context.Context, prefix, delimiter string) (pio.ObjectIterator, error)
}

// DefaultFile describes a file that should exist in ArtifactOpts.Dir.
// If the file is not present, these values will be used instead.
type DefaultFile struct {
	Name string
	Size int64
}
type ArtifactOpts struct {
	// Dir is the top-level directory, including the provider, e.g.
	// "gs://some-bucket/path"; include all files here.
	Dir string
	// ArtifactsDirOnly includes only the "Dir/artifacts/" directory,
	// instead of files in the tree rooted there. Experimental.
	ArtifactsDirOnly bool
	// DefaultFiles are files in directory Dir (not nested) that are
	// included in the output if they don't exist.
	DefaultFiles []DefaultFile
}

// ArtifactFiles returns the files based on ArtifactOpts.
//
// In the event of error, returns any files collected so far in the
// interest of best effort.
func ArtifactFiles(ctx context.Context, opener fileFinder, o ArtifactOpts) ([]*resultstore.File, error) {
	prefix := ensureTrailingSlash(o.Dir)
	c, err := newFilesCollector(opener, prefix)
	if err != nil {
		return nil, err
	}

	// Collect the files in the top-level dir.
	if err := c.collect(ctx, prefix, "/"); err != nil {
		return c.builder.files, err
	}

	c.addDefaultFiles(prefix, o.DefaultFiles)

	if o.ArtifactsDirOnly {
		artifacts := prefix + "artifacts/"
		match := func(name string) bool {
			fmt.Printf("\nname: %q\n", name)
			return name == artifacts
		}
		if err := c.collectDirs(ctx, prefix, match); err != nil {
			return c.builder.files, err
		}
		return c.builder.files, nil
	}

	// Collect the entire artifacts/ subtree.
	if err := c.collect(ctx, prefix+"artifacts/", ""); err != nil {
		return c.builder.files, err
	}
	return c.builder.files, nil
}

func ensureTrailingSlash(p string) string {
	if strings.HasSuffix(p, "/") {
		return p
	}
	return p + "/"
}

type filesCollector struct {
	finder fileFinder
	// The bucket, including provider, e.g. "gs://some-bucket/".
	bucket  string
	builder *filesBuilder
}

// bucket returns a string of the provider and bucket name, with a
// trailing slash.
func bucket(path string) (string, error) {
	provider, bucket, _, err := providers.ParseStoragePath(path)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s://%s/", provider, bucket), nil
}

func newFilesCollector(opener fileFinder, prefix string) (*filesCollector, error) {
	b, err := bucket(prefix)
	if err != nil {
		return nil, err
	}
	return &filesCollector{
		finder:  opener,
		bucket:  b,
		builder: newFilesBuilder(prefix),
	}, nil
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
		c.builder.Add(c.bucket+f.Name, f.Size)
	}
	return nil
}

// addDefaultFiles adds default files if not already collected.
func (c *filesCollector) addDefaultFiles(prefix string, files []DefaultFile) {
	if len(files) == 0 {
		return
	}
	seen := map[string]bool{}
	for _, f := range c.builder.files {
		seen[f.Uri] = true
	}
	for _, f := range files {
		name := prefix + f.Name
		if seen[name] {
			continue
		}
		c.builder.Add(name, f.Size)
	}
}

// collectDirs collects directories in prefix where match is true.
func (c *filesCollector) collectDirs(ctx context.Context, prefix string, match func(string) bool) error {
	iter, err := c.finder.Iterator(ctx, prefix, "/")
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
		if !f.IsDir {
			continue
		}
		name := c.bucket + f.Name
		if match(name) {
			c.builder.AddDir(name)
		}
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

func (b *filesBuilder) Add(name string, size int64) {
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
		Length:      &wrapperspb.Int64Value{Value: size},
		ContentType: contentType(uid),
	})
}

func (b *filesBuilder) AddDir(name string) {
	uid := b.trim(name)
	b.files = append(b.files, &resultstore.File{
		Uid: uid,
		Uri: name,
	})
}

func init() {
	// Avoid the default of "text/x-log" for log files.
	mime.AddExtensionType(".log", "text/plain")
	// May not exist in the container.
	mime.AddExtensionType(".txt", "text/plain")
}

func contentType(name string) string {
	ps := strings.SplitN(mime.TypeByExtension(filepath.Ext(name)), ";", 2)
	return ps[0]
}
