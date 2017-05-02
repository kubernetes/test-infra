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
	"fmt"
	"os"
	"testing"
	"time"

	"cloud.google.com/go/storage"
)

func TestFederationLockE2E(t *testing.T) {
	bucket := os.Getenv("FEDERATION_LOCK_TEST_BUCKET")
	if bucket == "" {
		t.Skip("FEDERATION_LOCK_TEST_BUCKET environment variable is empty")
	}

	ctx, obj, err := createTestParams(bucket, lockFile)
	if err != nil {
		t.Fatalf("failed to initialize test environment: %v", err)
	}
	defer removeLockObject(ctx, obj)

	// Set build number
	buildNum := "5"
	os.Setenv("BUILD_NUMBER", buildNum)

	// recycle clusters
	o1 := &options{
		federation: false,
		up:         true,
		deployment: "bash",
		save:       bucket,
	}

	// reuse clusters
	o2 := &options{
		federation: true,
		up:         true,
		deployment: "none",
		save:       bucket,
	}

	t.Run("Recycle-Reuse", func(t *testing.T) {
		// add barrier
		err = prepareFederation(o1)
		if err != nil {
			t.Fatalf("failed to prepare federation test environment: %v", err)
		}
		content, err := readObject(ctx, obj)
		if !bytes.Equal(content, barrier) {
			t.Fatalf("barrier wasn't added to the object: want: %q, got: %q", string(barrier), string(content))
		}

		// remove barrier
		err = cleanupFederation(o1)
		if err != nil {
			t.Fatalf("failed to cleanup federation test environment: %v", err)
		}
		content, err = readObject(ctx, obj)
		if len(content) != 0 {
			t.Fatalf("want: (empty object file), got: %s", string(content))
		}

		// lock object
		err = prepareFederation(o2)
		if err != nil {
			t.Fatalf("failed to prepare federation test environment: %v", err)
		}
		content, err = readObject(ctx, obj)
		if !bytes.HasPrefix(content, []byte(buildNum)) {
			t.Fatalf("lock wasn't added to the object: want: %q, got: %q", buildNum, string(content))
		}

		// unlock object
		err = cleanupFederation(o2)
		if err != nil {
			t.Fatalf("failed to cleanup federation test environment: %v", err)
		}
		content, err = readObject(ctx, obj)
		if len(content) != 0 {
			t.Fatalf("want: (empty object file), got: %s", string(content))
		}
	})

	t.Run("Reuse-Recycle", func(t *testing.T) {
		// lock object
		err = prepareFederation(o2)
		if err != nil {
			t.Fatalf("failed to prepare federation test environment: %v", err)
		}
		content, err := readObject(ctx, obj)
		if !bytes.HasPrefix(content, []byte(buildNum)) {
			t.Fatalf("lock wasn't added to the object: want: %q, got: %q", buildNum, string(content))
		}

		// unlock object
		err = cleanupFederation(o2)
		if err != nil {
			t.Fatalf("failed to cleanup federation test environment: %v", err)
		}
		content, err = readObject(ctx, obj)
		if len(content) != 0 {
			t.Fatalf("want: (empty object file), got: %s", string(content))
		}

		// add barrier
		err = prepareFederation(o1)
		if err != nil {
			t.Fatalf("failed to prepare federation test environment: %v", err)
		}
		content, err = readObject(ctx, obj)
		if !bytes.Equal(content, barrier) {
			t.Fatalf("barrier wasn't added to the object: want: %q, got: %q", string(barrier), string(content))
		}

		// remove barrier
		err = cleanupFederation(o1)
		if err != nil {
			t.Fatalf("failed to cleanup federation test environment: %v", err)
		}
		content, err = readObject(ctx, obj)
		if len(content) != 0 {
			t.Fatalf("want: (empty object file), got: %s", string(content))
		}
	})
}

func TestGCSLock(t *testing.T) {
	bucket := os.Getenv("FEDERATION_LOCK_TEST_BUCKET")
	if bucket == "" {
		t.Skip("FEDERATION_LOCK_TEST_BUCKET environment variable is empty")
	}

	ctx, obj, err := createTestParams(bucket, lockFile)
	if err != nil {
		t.Fatalf("failed to initialize test environment: %v", err)
	}
	defer removeLockObject(ctx, obj)

	// Set build number
	buildNum := "5"
	os.Setenv("BUILD_NUMBER", buildNum)

	l, err := newLock(bucket, lockFile)
	if err != nil {
		t.Fatalf("failed to create :%v", err)
	}

	t.Run("Lock-Barrier", func(t *testing.T) {
		err = l.lock(buildNum)
		if err != nil {
			t.Fatalf("failed to acquire lock: %v", err)
		}
		content, err := readObject(ctx, obj)
		if !bytes.HasPrefix(content, []byte(buildNum)) {
			t.Fatalf("lock wasn't added to the object: want: %q, got: %q", buildNum, string(content))
		}

		err = l.unlock(buildNum)
		if err != nil {
			t.Fatalf("failed to release lock: %v", err)
		}
		content, err = readObject(ctx, obj)
		if len(content) != 0 {
			t.Fatalf("want: (empty object file), got: %s", string(content))
		}

		err = l.addBarrier()
		if err != nil {
			t.Fatalf("failed to add barrier: %v", err)
		}
		content, err = readObject(ctx, obj)
		if !bytes.Equal(content, barrier) {
			t.Fatalf("barrier wasn't added to the object: want: %q, got: %q", string(barrier), string(content))
		}

		err = l.waitForEmptyBarrier()
		if err != nil {
			t.Fatalf("failed to wait for empty barrier: %v", err)
		}
		content, err = readObject(ctx, obj)
		if !bytes.Equal(content, barrier) {
			t.Fatalf("barrier doesn't exist in the object: want: %q, got: %q", string(barrier), string(content))
		}

		err = l.removeBarrier()
		if err != nil {
			t.Fatalf("failed to remove barrier: %v", err)
		}
		content, err = readObject(ctx, obj)
		if len(content) != 0 {
			t.Fatalf("want: (empty object file), got: %s", string(content))
		}
	})

	t.Run("Barrier-Lock", func(t *testing.T) {
		err = l.addBarrier()
		if err != nil {
			t.Fatalf("failed to add barrier: %v", err)
		}
		content, err := readObject(ctx, obj)
		if !bytes.Equal(content, barrier) {
			t.Fatalf("barrier wasn't added to the object: want: %q, got: %q", string(barrier), string(content))
		}

		err = l.waitForEmptyBarrier()
		if err != nil {
			t.Fatalf("failed to wait for empty barrier: %v", err)
		}
		content, err = readObject(ctx, obj)
		if !bytes.Equal(content, barrier) {
			t.Fatalf("barrier doesn't exist in the object: want: %q, got: %q", string(barrier), string(content))
		}

		err = l.removeBarrier()
		if err != nil {
			t.Fatalf("failed to remove barrier: %v", err)
		}
		content, err = readObject(ctx, obj)
		if len(content) != 0 {
			t.Fatalf("want: (empty object file), got: %s", string(content))
		}

		err = l.lock(buildNum)
		if err != nil {
			t.Fatalf("failed to acquire lock: %v", err)
		}
		content, err = readObject(ctx, obj)
		if !bytes.HasPrefix(content, []byte(buildNum)) {
			t.Fatalf("lock wasn't added to the object: want: %q, got: %q", buildNum, string(content))
		}

		err = l.unlock(buildNum)
		if err != nil {
			t.Fatalf("failed to release lock: %v", err)
		}
		content, err = readObject(ctx, obj)
		if len(content) != 0 {
			t.Fatalf("want: (empty object file), got: %s", string(content))
		}
	})

	t.Run("Lock-AddBarrier-Unlock-Wait-RemoveBarrier", func(t *testing.T) {
		err = l.lock(buildNum)
		if err != nil {
			t.Fatalf("failed to acquire lock: %v", err)
		}
		content, err := readObject(ctx, obj)
		if !bytes.HasPrefix(content, []byte(buildNum)) {
			t.Fatalf("lock wasn't added to the object: want: %q, got: %q", buildNum, string(content))
		}

		err = l.addBarrier()
		if err != nil {
			t.Fatalf("failed to add barrier: %v", err)
		}
		want := append(content, barrier...)
		content, err = readObject(ctx, obj)
		if !bytes.Equal(content, want) {
			t.Fatalf("barrier doesn't exist in the object: want: %q, got: %q", string(want), string(content))
		}

		err = l.unlock(buildNum)
		if err != nil {
			t.Fatalf("failed to release lock: %v", err)
		}
		content, err = readObject(ctx, obj)
		if !bytes.Equal(content, barrier) {
			t.Fatalf("lock object isn't removed: want: %q, got: %q", string(barrier), string(content))
		}

		err = l.waitForEmptyBarrier()
		if err != nil {
			t.Fatalf("failed to wait for empty barrier: %v", err)
		}
		content, err = readObject(ctx, obj)
		if !bytes.Equal(content, barrier) {
			t.Fatalf("barrier object doesn't exist: want: %q, got: %q", string(barrier), string(content))
		}

		err = l.removeBarrier()
		if err != nil {
			t.Fatalf("failed to remove barrier: %v", err)
		}
		content, err = readObject(ctx, obj)
		if len(content) != 0 {
			t.Fatalf("want: (empty object file), got: %s", string(content))
		}
	})

	t.Run("Lock-AddBarrier-Concurrent-Unlock-Wait-RemoveBarrier", func(t *testing.T) {
		err = l.lock(buildNum)
		if err != nil {
			t.Fatalf("failed to acquire lock: %v", err)
		}
		content, err := readObject(ctx, obj)
		if !bytes.HasPrefix(content, []byte(buildNum)) {
			t.Fatalf("lock wasn't added to the object: want: %q, got: %q", buildNum, string(content))
		}

		err = l.addBarrier()
		if err != nil {
			t.Fatalf("failed to add barrier: %v", err)
		}
		want := append(content, barrier...)
		content, err = readObject(ctx, obj)
		if !bytes.Equal(content, want) {
			t.Fatalf("barrier doesn't exist in the object: want: %q, got: %q", string(want), string(content))
		}

		ch := make(chan struct{})
		go func() {
			<-ch
			time.Sleep(5 * pollInterval)
			err = l.unlock(buildNum)
			if err != nil {
				t.Fatalf("failed to release lock: %v", err)
			}
			content, err = readObject(ctx, obj)
			if !bytes.Equal(content, barrier) {
				t.Fatalf("lock object isn't removed: want: %q, got: %q", string(barrier), string(content))
			}
		}()

		ch <- struct{}{}
		err = l.waitForEmptyBarrier()
		if err != nil {
			t.Fatalf("failed to wait for empty barrier: %v", err)
		}
		content, err = readObject(ctx, obj)
		if !bytes.Equal(content, barrier) {
			t.Fatalf("barrier object doesn't exist: want: %q, got: %q", string(barrier), string(content))
		}

		err = l.removeBarrier()
		if err != nil {
			t.Fatalf("failed to remove barrier: %v", err)
		}
		content, err = readObject(ctx, obj)
		if len(content) != 0 {
			t.Fatalf("want: (empty object file), got: %s", string(content))
		}
	})
}

// removeLockObject atomically deletes the lock object from GCS.
func removeLockObject(ctx context.Context, obj *storage.ObjectHandle) error {
	return obj.Delete(ctx)
}

func createTestParams(save, lock string) (context.Context, *storage.ObjectHandle, error) {
	pollInterval = 1 * time.Second
	pollTimeout = 10 * time.Second

	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create storage client: %v", err)
	}

	bucket, objName := parseBucketObject(save, lock)

	return ctx, client.Bucket(bucket).Object(objName), nil
}
