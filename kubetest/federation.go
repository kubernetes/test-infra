/*
Copyright 2017 The Kubernetes Authors.

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

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os/exec"
	"path"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/api/googleapi"
)

const (
	conditionNotMet = "conditionNotMet"

	lockFile = "federated-clusters.lock"
)

var (
	pollInterval = 1 * time.Minute
	pollTimeout  = 1 * time.Hour

	barrier = []byte("deploy barrier\n")

	errWaitTimeout = errors.New("timed out waiting for the condition")
)

func FedUp() error {
	return finishRunning(exec.Command("./federation/cluster/federation-up.sh"))
}

func FedDown() error {
	return finishRunning(exec.Command("./federation/cluster/federation-down.sh"))
}

func parseBucketObject(base, name string) (string, string) {
	dir := strings.TrimPrefix(base, "gs://")
	splits := strings.SplitN(dir, "/", 2)
	if len(splits) == 1 {
		return splits[0], name
	}
	return splits[0], path.Join(splits[1], name)
}

type gcsLock struct {
	bucket string
	obj    string
	ctx    context.Context
	client *storage.Client
}

func newLock(save, lock string) (*gcsLock, error) {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage client: %v", err)
	}

	bucket, obj := parseBucketObject(save, lock)
	return &gcsLock{
		bucket: bucket,
		obj:    obj,
		ctx:    ctx,
		client: client,
	}, nil
}

func (l *gcsLock) lock(data string) error {
	log.Printf("Acquiring lock: %s", data)

	line := fmt.Sprintf("%s\t%s\n", data, time.Now())

	return l.readModifyWrite(func(content []byte) ([]byte, bool) {
		if bytes.HasSuffix(content, barrier) {
			return nil, true
		}
		return append(content, []byte(line)...), false
	})
}

func (l *gcsLock) unlock(data string) error {
	log.Printf("Releasing lock: %s", data)

	return l.readModifyWrite(func(content []byte) ([]byte, bool) {
		return removeLineWithPrefix(content, []byte(data)), false
	})
}

func (l *gcsLock) addBarrier() error {
	log.Printf("Adding barrier")

	return l.readModifyWrite(func(content []byte) ([]byte, bool) {
		return append(content, []byte(barrier)...), false
	})
}

func (l *gcsLock) waitForEmptyBarrier() error {
	log.Printf("Waiting for empty barrier")

	obj := l.client.Bucket(l.bucket).Object(l.obj)

	return poll(func() (bool, error) {
		content, err := readObject(l.ctx, obj)
		if err != nil {
			return false, fmt.Errorf("failed to read object %s/%s: %v", l.bucket, l.obj, err)
		}
		if bytes.Equal(content, barrier) {
			return true, nil
		}
		return false, nil
	})
}

func (l *gcsLock) removeBarrier() error {
	log.Printf("Removing barrier")

	return l.readModifyWrite(func(content []byte) ([]byte, bool) {
		return removeLineWithPrefix(content, barrier), false
	})
}

func (l *gcsLock) readModifyWrite(modify func([]byte) ([]byte, bool)) error {
	obj := l.client.Bucket(l.bucket).Object(l.obj)

	return poll(func() (bool, error) {
		log.Print("Read-Modify-Write")

		attrs, err := obj.Attrs(l.ctx)
		if err != nil && err != storage.ErrObjectNotExist {
			return false, fmt.Errorf("failed to retrieve attributes for %s/%s: (%T)%v", l.bucket, l.obj, err, err)
		}

		content := make([]byte, 0)
		conditions := storage.Conditions{}
		if err == storage.ErrObjectNotExist {
			conditions.DoesNotExist = true
			log.Printf("object %s/%s doesn't exist, will be created in the next write cycle", l.bucket, l.obj)
		} else {
			conditions.GenerationMatch = attrs.Generation

			log.Printf("reading object %s/%s", l.bucket, l.obj)
			content, err = readObject(l.ctx, obj)
			if err != nil {
				return false, fmt.Errorf("failed to read object %s/%s: %v", l.bucket, l.obj, err)
			}
		}

		newContent, retry := modify(content)
		if retry {
			return false, nil
		}

		w := obj.If(conditions).NewWriter(l.ctx)
		if _, err := w.Write(newContent); err != nil {
			return false, fmt.Errorf("failed to write the object %s/%s: %v", l.bucket, l.obj, err)
		}

		err = w.Close()
		if apiErr, ok := err.(*googleapi.Error); ok {
			if apiErr.Code == http.StatusPreconditionFailed && apiErr.Errors[0].Reason == conditionNotMet {
				return false, nil
			} else {
				return false, fmt.Errorf("failed to close the writer for %s/%s: %v", l.bucket, l.obj, apiErr)
			}
		} else if err != nil {
			return false, fmt.Errorf("failed to close the writer for %s/%s: %v", l.bucket, l.obj, err)
		}
		return true, nil
	})
}

func poll(cond func() (bool, error)) error {
	ok, err := cond()
	if err != nil {
		return err
	}
	if ok {
		return nil
	}

	interval := time.NewTicker(pollInterval)
	defer interval.Stop()

	timeout := time.NewTimer(pollTimeout)
	defer timeout.Stop()

	for {
		select {
		case <-interval.C:
			ok, err := cond()
			if err != nil {
				return err
			}
			if ok {
				return nil
			}
		case <-timeout.C:
			return errWaitTimeout
		}
	}

	return fmt.Errorf("unknown poll error")
}

func readObject(ctx context.Context, obj *storage.ObjectHandle) ([]byte, error) {
	r, err := obj.NewReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize reader: %v", err)
	}
	defer r.Close()

	return ioutil.ReadAll(r)
}

func removeLineWithPrefix(content []byte, prefix []byte) []byte {
	lines := bytes.Split(content, []byte("\n"))
	if prefix[len(prefix)-1] == '\n' {
		prefix = prefix[:len(prefix)-1]
	}

	newContent := []byte{}
	for _, line := range lines {
		if bytes.HasPrefix(line, prefix) {
			continue
		}
		if len(line) > 0 {
			newContent = append(newContent, line...)
			newContent = append(newContent, '\n')
		}
	}

	return newContent
}
