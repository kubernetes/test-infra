/*
Copyright 2019 The Kubernetes Authors.

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

package gcs

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"regexp"
	"sync"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"

	"k8s.io/test-infra/testgrid/metadata"
	"k8s.io/test-infra/testgrid/metadata/junit"
)

// Started holds started.json data.
type Started = metadata.Started

// Finished holds finished.json data.
type Finished struct {
	metadata.Finished
	Running bool
}

// Build points to a build stored under a particular gcs prefix.
type Build struct {
	Bucket  *storage.BucketHandle
	Context context.Context
	Prefix  string
}

func (build Build) String() string {
	return build.Prefix
}

// junit_CONTEXT_TIMESTAMP_THREAD.xml
var re = regexp.MustCompile(`.+/junit(_[^_]+)?(_\d+-\d+)?(_\d+)?\.xml$`)

// dropPrefix removes the _ in _CONTEXT to help keep the regexp simple
func dropPrefix(name string) string {
	if len(name) == 0 {
		return name
	}
	return name[1:]
}

// parseSuitesMeta returns the metadata for this junit file (nil for a non-junit file).
//
// Expected format: junit_context_20180102-1256-07.xml
// Results in {
//   "Context": "context",
//   "Timestamp": "20180102-1256",
//   "Thread": "07",
// }
func parseSuitesMeta(name string) map[string]string {
	mat := re.FindStringSubmatch(name)
	if mat == nil {
		return nil
	}
	return map[string]string{
		"Context":   dropPrefix(mat[1]),
		"Timestamp": dropPrefix(mat[2]),
		"Thread":    dropPrefix(mat[3]),
	}

}

// readJSON will decode the json object stored in GCS.
func readJSON(ctx context.Context, obj *storage.ObjectHandle, i interface{}) error {
	reader, err := obj.NewReader(ctx)
	if err != nil {
		return fmt.Errorf("open: %v", err)
	}
	if err = json.NewDecoder(reader).Decode(i); err != nil {
		return fmt.Errorf("decode: %v", err)
	}
	return nil
}

// Started parses the build's started metadata.
func (build Build) Started() (*Started, error) {
	uri := build.Prefix + "started.json"
	var started Started
	if err := readJSON(build.Context, build.Bucket.Object(uri), &started); err != nil {
		return nil, fmt.Errorf("read %s: %v", uri, err)
	}
	return &started, nil
}

// Finished parses the build's finished metadata.
func (build Build) Finished() (*Finished, error) {
	uri := build.Prefix + "finished.json"
	var finished Finished
	if err := readJSON(build.Context, build.Bucket.Object(uri), &finished); err != nil {
		return nil, fmt.Errorf("read %s: %v", uri, err)
	}
	return &finished, nil
}

// Artifacts writes the object name of all paths under the build's artifact dir to the output channel.
func (build Build) Artifacts(artifacts chan<- string) error {
	pref := build.Prefix + "artifacts/"
	objs := build.Bucket.Objects(build.Context, &storage.Query{Prefix: pref})
	for {
		obj, err := objs.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to list %s: %v", pref, err)
		}
		select {
		case <-build.Context.Done():
			return fmt.Errorf("interrupted listing %s", pref)
		case artifacts <- obj.Name:
		}
	}
	return nil
}

// readSuites parses the <testsuite> or <testsuites> object in obj
func readSuites(ctx context.Context, obj *storage.ObjectHandle) (*junit.Suites, error) {
	reader, err := obj.NewReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("open: %v", err)
	}

	buf, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read: %v", err)
	}

	suites, err := junit.Parse(buf)
	if err != nil {
		return nil, fmt.Errorf("parse: %v", err)
	}
	return &suites, nil
}

// SuitesMeta holds testsuites xml and metadata from the filename
type SuitesMeta struct {
	Suites   junit.Suites      // suites data extracted from file contents
	Metadata map[string]string // metadata extracted from path name
}

// Suites takes a channel of artifact names, parses those representing junit suites, writing the result to the suites channel.
//
// Note that junit suites are parsed in parallel, so there are no guarantees about suites ordering.
func (build Build) Suites(artifacts <-chan string, suites chan<- SuitesMeta) error {

	var wg sync.WaitGroup
	ec := make(chan error)
	ctx, cancel := context.WithCancel(build.Context)
	defer cancel()
	for art := range artifacts {
		meta := parseSuitesMeta(art)
		if meta == nil {
			continue // not a junit file ignore it, ignore it
		}
		wg.Add(1)
		// concurrently parse each file because there may be a lot of them, and
		// each takes a non-trivial amount of time waiting for the network.
		go func(art string, meta map[string]string) {
			defer wg.Done()
			suitesData, err := readSuites(ctx, build.Bucket.Object(art))
			if err != nil {
				select {
				case <-ctx.Done():
				case ec <- err:
				}
				return
			}
			out := SuitesMeta{
				Suites:   *suitesData,
				Metadata: meta,
			}
			select {
			case <-ctx.Done():
			case suites <- out:
			}
		}(art, meta)
	}

	go func() {
		wg.Wait()
		select {
		case ec <- nil: // tell parent we exited cleanly
		case <-ctx.Done(): // parent already exited
		}
		close(ec) // no one will send t
	}()

	select {
	case <-ctx.Done(): // parent context marked as finished.
		return ctx.Err()
	case err := <-ec: // finished listing
		return err
	}
}
