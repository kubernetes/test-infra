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
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
	"golang.org/x/sync/semaphore"
	kerrors "k8s.io/apimachinery/pkg/util/errors"

	"k8s.io/test-infra/prow/secretutil"
)

// defaultBufferSize is the default buffer size, 10MiB.
const defaultBufferSize = 10 * 1024 * 1024

func (o Options) censor() error {
	var concurrency int64
	if o.CensoringConcurrency == nil {
		concurrency = int64(10)
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
	censorFile := fileCensorer(sem, errors, censorer, bufferSize)
	censor := func(file string) {
		censorFile(wg, file)
	}

	for _, entry := range o.Entries {
		logPath := entry.ProcessLog
		censor(logPath)
	}

	for _, item := range o.GcsOptions.Items {
		if err := filepath.Walk(item, func(absPath string, info os.FileInfo, err error) error {
			if info.IsDir() {
				return nil
			}

			contentType, err := determineContentType(absPath)
			if err != nil {
				return fmt.Errorf("could not determine content type of %s: %w", absPath, err)
			}

			switch contentType {
			case "application/x-gzip", "application/zip":
				if err := handleArchive(absPath, censorFile); err != nil {
					return fmt.Errorf("could not censor archive %s: %w", absPath, err)
				}
			default:
				censor(absPath)
			}
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

// fileCensorer returns a closure over all of our synchronization for a clean handler signature
func fileCensorer(sem *semaphore.Weighted, errors chan<- error, censorer secretutil.Censorer, bufferSize int) func(wg *sync.WaitGroup, file string) {
	return func(wg *sync.WaitGroup, file string) {
		wg.Add(1)
		go func() {
			if err := sem.Acquire(context.Background(), 1); err != nil {
				errors <- err
				return
			}
			defer sem.Release(1)
			defer wg.Done()
			errors <- handleFile(file, censorer, bufferSize)
		}()
	}
}

// determineContentType determines the content type of the file
func determineContentType(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("could not open file to check content type: %v", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			logrus.WithError(err).Warn("Could not close input file while detecting content type.")
		}
	}()

	header := make([]byte, 512)
	if _, err := file.Read(header); err != nil {
		return "", fmt.Errorf("could not read file to check content type: %v", err)
	}
	return http.DetectContentType(header), nil
}

// handleArchive unravels the archive in order to censor data in the files that were added to it.
// This is mostly stolen from build/internal/untar/untar.go
func handleArchive(archivePath string, censor func(wg *sync.WaitGroup, file string)) error {
	outputDir, err := ioutil.TempDir("", "tmp-unpack")
	if err != nil {
		return fmt.Errorf("could not create temporary dir for unpacking: %w", err)
	}

	defer func() {
		if err := os.RemoveAll(outputDir); err != nil {
			logrus.WithError(err).Warn("Failed to clean up temporary directory for archive")
		}
	}()

	if err := unarchive(archivePath, outputDir); err != nil {
		return fmt.Errorf("could not unpack archive: %w", err)
	}

	children := &sync.WaitGroup{}
	if err := filepath.Walk(outputDir, func(absPath string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}

		censor(children, absPath)
		return nil
	}); err != nil {
		return fmt.Errorf("could not walk unpacked archive to censor them: %w", err)
	}

	children.Wait()
	if err := archive(outputDir, archivePath); err != nil {
		return fmt.Errorf("could not re-pack archive: %w", err)
	}
	return nil
}

// unarchive unpacks the archive into the destination
func unarchive(archivePath, destPath string) error {
	input, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("could not open archive for unpacking: %w", err)
	}
	zipReader, err := gzip.NewReader(input)
	if err != nil {
		return fmt.Errorf("could not read archive: %w", err)
	}
	tarReader := tar.NewReader(zipReader)
	defer func() {
		if err := zipReader.Close(); err != nil {
			logrus.WithError(err).Warn("Could not close zip reader after unarchiving.")
		}
		if err := input.Close(); err != nil {
			logrus.WithError(err).Warn("Could not close input file after unarchiving.")
		}
	}()

	for {
		entry, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("could not read archive: %w", err)
		}
		if !validRelPath(entry.Name) {
			return fmt.Errorf("tar contained invalid name error %q", entry.Name)
		}
		rel := filepath.FromSlash(entry.Name)
		abs := filepath.Join(destPath, rel)
		mode := entry.FileInfo().Mode()
		switch {
		case mode.IsDir():
			if err := os.MkdirAll(abs, 0755); err != nil {
				return fmt.Errorf("could not create directory while unpacking archive: %w", err)
			}
		case mode.IsRegular():
			file, err := os.OpenFile(abs, os.O_RDWR|os.O_CREATE|os.O_TRUNC, mode.Perm())
			if err != nil {
				return err
			}
			n, err := io.Copy(file, tarReader)
			if closeErr := file.Close(); closeErr != nil && err == nil {
				return fmt.Errorf("error closing %s: %v", abs, closeErr)
			}
			if err != nil {
				return fmt.Errorf("error writing to %s: %v", abs, err)
			}
			if n != entry.Size {
				return fmt.Errorf("only wrote %d bytes to %s; expected %d", n, abs, entry.Size)
			}
		}
	}
	return nil
}

func validRelPath(p string) bool {
	if p == "" || strings.Contains(p, `\`) || strings.HasPrefix(p, "/") || strings.Contains(p, "../") {
		return false
	}
	return true
}

// archive re-packs the dir into the destination
func archive(srcDir, destArchive string) error {
	output, err := ioutil.TempFile("", "tmp-archive")
	if err != nil {
		return fmt.Errorf("failed to create temporary file for archive: %w", err)
	}

	zipWriter := gzip.NewWriter(output)
	tarWriter := tar.NewWriter(zipWriter)
	defer func() {
		if err := tarWriter.Close(); err != nil {
			logrus.WithError(err).Warn("Could not close tar writer after archiving.")
		}
		if err := zipWriter.Close(); err != nil {
			logrus.WithError(err).Warn("Could not close zip writer after archiving.")
		}
		if err := output.Close(); err != nil {
			logrus.WithError(err).Warn("Could not close output file after archiving.")
		}
	}()

	if err := filepath.Walk(srcDir, func(absPath string, info os.FileInfo, err error) error {
		header, err := tar.FileInfoHeader(info, info.Name())
		if err != nil {
			return fmt.Errorf("could not create tar header: %w", err)
		}
		// the header won't get nested paths right
		relpath, _ := filepath.Rel(srcDir, absPath) // err happens when there's no rel path, but we know there must be
		header.Name = relpath
		if err := tarWriter.WriteHeader(header); err != nil {
			return fmt.Errorf("could not write tar header: %w", err)
		}
		if info.IsDir() {
			return nil
		}
		file, err := os.Open(absPath)
		if err != nil {
			return fmt.Errorf("could not open source file: %w", err)
		}
		n, err := io.Copy(tarWriter, file)
		if err != nil {
			return fmt.Errorf("could not tar file: %w", err)
		}
		if n != info.Size() {
			return fmt.Errorf("only wrote %d bytes from %s; expected %d", n, absPath, info.Size())
		}
		if err := file.Close(); err != nil {
			return fmt.Errorf("could not close source file: %w", err)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("could not walk source files to archive them: %w", err)
	}

	if err := os.Rename(output.Name(), destArchive); err != nil {
		return fmt.Errorf("could not overwrite archive: %w", err)
	}

	return nil
}

// handleFile censors the content of a file by streaming it to a new location, then overwriting the previous
// location, to make it seem like this happened in place on the filesystem
func handleFile(path string, censorer secretutil.Censorer, bufferSize int) error {
	input, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("could not open file for censoring: %w", err)
	}

	output, err := ioutil.TempFile("", "tmp-censor")
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
