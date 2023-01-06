/*
Copyright 2022 The Kubernetes Authors.

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

package fake

import (
	"bytes"
	"context"
	"io"
)

type Artifact struct {
	Path    string
	Content []byte
	Meta    map[string]string
	Link    *string
}

func (fa *Artifact) JobPath() string {
	return fa.Path
}

func (fa *Artifact) Size() (int64, error) {
	return int64(len(fa.Content)), nil
}

func (fa *Artifact) Metadata() (map[string]string, error) {
	return fa.Meta, nil
}

func (fa *Artifact) UpdateMetadata(now map[string]string) error {
	fa.Meta = now
	return nil
}

const NotFound = "linknotfound.io/404"

func (fa *Artifact) CanonicalLink() string {
	if fa.Link == nil {
		return NotFound
	}
	return *fa.Link
}

func (fa *Artifact) ReadAt(b []byte, off int64) (int, error) {
	r := bytes.NewReader(fa.Content)
	return r.ReadAt(b, off)
}

func (fa *Artifact) ReadAll() ([]byte, error) {
	r := bytes.NewReader(fa.Content)
	return io.ReadAll(r)
}

func (fa *Artifact) ReadTail(n int64) ([]byte, error) {
	size, err := fa.Size()
	if err != nil {
		return nil, err
	}
	buf := make([]byte, n)
	_, err = fa.ReadAt(buf, size-n)
	return buf, err
}

func (fa *Artifact) UseContext(ctx context.Context) error {
	return nil
}

func (fa *Artifact) ReadAtMost(n int64) ([]byte, error) {
	buf := make([]byte, n)
	_, err := fa.ReadAt(buf, 0)
	return buf, err
}
