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

package lenses

import (
	"bytes"
	"context"
	"io/ioutil"
)

type FakeArtifact struct {
	Path      string
	Content   []byte
	SizeLimit int64
}

func (fa *FakeArtifact) JobPath() string {
	return fa.Path
}

func (fa *FakeArtifact) Size() (int64, error) {
	return int64(len(fa.Content)), nil
}

func (fa *FakeArtifact) CanonicalLink() string {
	return "linknotfound.io/404"
}

func (fa *FakeArtifact) ReadAt(b []byte, off int64) (int, error) {
	r := bytes.NewReader(fa.Content)
	return r.ReadAt(b, off)
}

func (fa *FakeArtifact) ReadAll() ([]byte, error) {
	size, err := fa.Size()
	if err != nil {
		return nil, err
	}
	if size > fa.SizeLimit {
		return nil, ErrFileTooLarge
	}
	r := bytes.NewReader(fa.Content)
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
