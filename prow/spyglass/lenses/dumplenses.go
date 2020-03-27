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
