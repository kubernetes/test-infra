/*
Copyright 2021 The Kubernetes Authors.

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

package sidecar

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/sirupsen/logrus"
	"golang.org/x/sync/semaphore"
	kerrors "k8s.io/apimachinery/pkg/util/errors"

	"k8s.io/test-infra/prow/pod-utils/wrapper"
	"k8s.io/test-infra/prow/secretutil"
)

// defaultBufferSize is the default buffer size, 10MiB.
const defaultBufferSize = 10 * 1024 * 1024

func (o Options) censor() error {
	var concurrency int64
	if o.CensoringConcurrency == nil {
		concurrency = int64(runtime.GOMAXPROCS(0))
	} else {
		concurrency = *o.CensoringConcurrency
	}
	sem := semaphore.NewWeighted(concurrency)
	wg := &sync.WaitGroup{}
	errors := make(chan error)
	var errs []error
	errLock := &sync.Mutex{}
	go func() {
		errLock.Lock()
		for err := range errors {
			errs = append(errs, err)
		}
		errLock.Unlock()
	}()

	secrets, err := loadSecrets(o.SecretDirectories)
	if err != nil {
		return fmt.Errorf("could not load secrets: %w", err)
	}
	censorer := secretutil.NewCensorer()
	censorer.RefreshBytes(secrets...)

	bufferSize := defaultBufferSize
	if o.CensoringBufferSize != nil {
		bufferSize = *o.CensoringBufferSize
	}
	if largest := censorer.LargestSecret(); 2*largest > bufferSize {
		bufferSize = 2 * largest
	}

	for _, entry := range o.Entries {
		wg.Add(1)
		go func(opt wrapper.Options) {
			if err := sem.Acquire(context.Background(), 1); err != nil {
				errors <- err
				return
			}
			defer sem.Release(1)
			defer wg.Done()
			errors <- handleFile(opt.ProcessLog, censorer, bufferSize)
		}(entry)
	}

	for _, path := range o.GcsOptions.Items {
		if err := filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
			if info.IsDir() {
				return nil
			}
			wg.Add(1)
			go func(p string) {
				if err := sem.Acquire(context.Background(), 1); err != nil {
					errors <- err
					return
				}
				defer sem.Release(1)
				defer wg.Done()
				errors <- handleFile(p, censorer, bufferSize)
			}(path)
			return nil
		}); err != nil {
			return fmt.Errorf("could not walk items to censor them: %w", err)
		}
	}

	wg.Wait()
	close(errors)
	errLock.Lock()
	return kerrors.NewAggregate(errs)
}

// handleFile censors the content of a file by streaming it to a new location, then overwriting the previous
// location, to make it seem like this happened in place on the filesystem
func handleFile(path string, censorer secretutil.Censorer, bufferSize int) error {
	input, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("could not open file for censoring: %w", err)
	}

	output, err := ioutil.TempFile("", "")
	if err != nil {
		return fmt.Errorf("could not create temporary file for censoring: %w", err)
	}

	if err := censor(input, output, censorer, bufferSize); err != nil {
		return fmt.Errorf("could not censor file: %w", err)
	}

	if err := os.Rename(output.Name(), path); err != nil {
		return fmt.Errorf("could not overwrite file after censoring: %w", err)
	}

	return nil
}

// censor censors input data and streams it to the output. We have a memory footprint of bufferSize bytes.
func censor(input io.ReadCloser, output io.WriteCloser, censorer secretutil.Censorer, bufferSize int) error {
	if bufferSize%2 != 0 {
		return fmt.Errorf("frame size must be even, not %d", bufferSize)
	}
	defer func() {
		if err := input.Close(); err != nil {
			logrus.WithError(err).Warn("Could not close input file after censoring.")
		}
		if err := output.Close(); err != nil {
			logrus.WithError(err).Warn("Could not close output file after censoring.")
		}
	}()

	buffer := make([]byte, bufferSize)
	frameSize := bufferSize / 2
	// bootstrap the algorithm by reading in the first half-frame
	numInitialized, initializeErr := input.Read(buffer[:frameSize])
	// handle read errors - if we read everything in this init step, the next read will return 0, EOF and
	// we can flush appropriately as part of the process loop
	if initializeErr != nil && initializeErr != io.EOF {
		return fmt.Errorf("could not read data from input file before censoring: %w", initializeErr)
	}
	frameSize = numInitialized // this will normally be bufferSize/2 but will be smaller at the end of the file
	for {
		// populate the second half of the buffer with new data
		numRead, readErr := input.Read(buffer[frameSize:])
		if readErr != nil && readErr != io.EOF {
			return fmt.Errorf("could not read data from input file before censoring: %w", readErr)
		}
		// censor the full buffer and flush the first half to the output
		censorer.Censor(&buffer)
		numWritten, writeErr := output.Write(buffer[:frameSize])
		if writeErr != nil {
			return fmt.Errorf("could not write data to output file after censoring: %w", writeErr)
		}
		if numWritten != frameSize {
			// TODO: we could retry here I guess? When would a filesystem write less than expected and not error?
			return fmt.Errorf("only wrote %d out of %d bytes after censoring", numWritten, frameSize)
		}
		// shift the buffer over and get ready to repopulate the rest with new data
		copy(buffer[:numRead], buffer[frameSize:frameSize+numRead])
		frameSize = numRead
		if readErr == io.EOF {
			break
		}
	}
	return nil
}

// loadSecrets loads all files under the paths into memory
func loadSecrets(paths []string) ([][]byte, error) {
	var secrets [][]byte
	for _, path := range paths {
		if err := filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
			if info.IsDir() {
				return nil
			}
			raw, err := ioutil.ReadFile(path)
			if err != nil {
				return err
			}
			secrets = append(secrets, raw)
			return nil
		}); err != nil {
			return nil, err
		}
	}
	return secrets, nil
}
