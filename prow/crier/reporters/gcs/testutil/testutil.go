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
	"errors"
	"fmt"
	"io"

	"k8s.io/test-infra/prow/config"
	pkgio "k8s.io/test-infra/prow/io"
)

type Fca struct {
	C config.Config
}

func (ca Fca) Config() *config.Config {
	return &ca.C
}

type TestAuthor struct {
	Handlers map[string]map[string]*TestFileHandler
}

func (ta *TestAuthor) GetOrNewHandler(bucket, path string, create bool) *TestFileHandler {
	if ta.Handlers == nil {
		ta.Handlers = make(map[string]map[string]*TestFileHandler)
	}
	if _, ok := ta.Handlers[bucket]; !ok {
		ta.Handlers[bucket] = make(map[string]*TestFileHandler)
	}
	if _, ok := ta.Handlers[bucket][path]; !ok && create {
		ta.Handlers[bucket][path] = &TestFileHandler{}
	}
	return ta.Handlers[bucket][path]
}

type TestFileHandler struct {
	Bucket    string
	Path      string
	Content   []byte
	Offset    int
	Overwrite bool
	Closed    bool
}

type TestAuthorWriteCloser struct {
	handler *TestFileHandler
}

func (wc *TestAuthorWriteCloser) Write(p []byte) (int, error) {
	if len(wc.handler.Content) > 0 {
		if !wc.handler.Overwrite {
			return 0, fmt.Errorf("already exist: %w", pkgio.PreconditionFailedObjectAlreadyExists)
		}
		wc.handler.Content = make([]byte, 0)
	}
	wc.handler.Content = append(wc.handler.Content, p...)
	return len(p), nil
}

func (wc *TestAuthorWriteCloser) Close() error {
	wc.handler.Closed = true
	return nil
}

type TestAuthorReadCloser struct {
	handler *TestFileHandler
}

func (wc *TestAuthorReadCloser) Read(p []byte) (int, error) {
	if wc.handler == nil {
		return 0, errors.New("not exist")
	}
	if len(p) == 0 {
		return 0, nil
	}
	var count int
	for i := 0; i < len(p); i++ {
		if wc.handler.Offset == len(wc.handler.Content) {
			break
		}
		p[i] = wc.handler.Content[wc.handler.Offset]
		wc.handler.Offset++
		count++
	}
	if count == 0 {
		return count, io.EOF
	}
	return count, nil
}

func (wc *TestAuthorReadCloser) Close() error {
	wc.handler.Closed = true
	return nil
}

func (ta *TestAuthor) NewReader(ctx context.Context, bucket, path string) (io.ReadCloser, error) {
	return &TestAuthorReadCloser{handler: ta.GetOrNewHandler(bucket, path, false)}, nil
}

func (ta *TestAuthor) NewWriter(ctx context.Context, bucket, path string, overwrite bool) (io.WriteCloser, error) {
	handler := ta.GetOrNewHandler(bucket, path, true)
	handler.Overwrite = overwrite
	return &TestAuthorWriteCloser{handler: handler}, nil
}
