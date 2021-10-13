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

package testutil

import (
	"context"
	"fmt"
	"io"

	"k8s.io/test-infra/prow/config"
)

type Fca struct {
	C config.Config
}

func (ca Fca) Config() *config.Config {
	return &ca.C
}

type TestAuthor struct {
	AlreadyUsed bool
	Bucket      string
	Path        string
	Content     []byte
	Overwrite   bool
	Closed      bool
}

type TestAuthorWriteCloser struct {
	author *TestAuthor
}

func (wc *TestAuthorWriteCloser) Write(p []byte) (int, error) {
	wc.author.Content = append(wc.author.Content, p...)
	return len(p), nil
}

func (wc *TestAuthorWriteCloser) Close() error {
	wc.author.Closed = true
	return nil
}

func (ta *TestAuthor) NewWriter(ctx context.Context, bucket, path string, overwrite bool) (io.WriteCloser, error) {
	if ta.AlreadyUsed {
		panic(fmt.Sprintf("NewWriter called on testAuthor twice: first for %q/%q, now for %q/%q", ta.Bucket, ta.Path, bucket, path))
	}
	ta.AlreadyUsed = true
	ta.Bucket = bucket
	ta.Path = path
	ta.Overwrite = overwrite
	return &TestAuthorWriteCloser{author: ta}, nil
}
