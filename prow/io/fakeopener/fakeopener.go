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

package fakeopener

import (
	"bytes"
	"context"
	"os"

	pkgio "k8s.io/test-infra/prow/io"
)

type FakeOpener struct {
	pkgio.Opener
	Buffer     map[string]*bytes.Buffer
	ReadError  error
	WriteError error
}

type nopReadWriteCloser struct {
	*bytes.Buffer
}

func (nc *nopReadWriteCloser) Close() error {
	return nil
}

func (fo *FakeOpener) Reader(ctx context.Context, path string) (pkgio.ReadCloser, error) {
	if fo.ReadError != nil {
		return nil, fo.ReadError
	}
	if fo.Buffer == nil {
		fo.Buffer = make(map[string]*bytes.Buffer)
	}
	if _, ok := fo.Buffer[path]; !ok {
		return nil, os.ErrNotExist
	}
	// The underlying Buffer contains a private `off(set)` field, which will
	// move to the end of the buffer after one read, and there is no way to
	// reset the `off(set)`. So create a new Buffer from the content, to make
	// this Buffer readable repeatedly.
	newBuf := bytes.NewBuffer(fo.Buffer[path].Bytes())
	return &nopReadWriteCloser{Buffer: newBuf}, nil
}

func (fo *FakeOpener) Writer(ctx context.Context, path string, opts ...pkgio.WriterOptions) (pkgio.WriteCloser, error) {
	if fo.WriteError != nil {
		return nil, fo.WriteError
	}
	if fo.Buffer == nil {
		fo.Buffer = make(map[string]*bytes.Buffer)
	}

	overWrite := true
	for _, o := range opts {
		if o.PreconditionDoesNotExist != nil && *o.PreconditionDoesNotExist {
			overWrite = false
			break
		}
	}
	if fo.Buffer[path] != nil {
		if !overWrite {
			return nil, os.ErrExist
		}
		fo.Buffer[path] = &bytes.Buffer{}
	}

	if _, ok := fo.Buffer[path]; !ok {
		fo.Buffer[path] = &bytes.Buffer{}
	}

	return &nopReadWriteCloser{Buffer: fo.Buffer[path]}, nil
}
