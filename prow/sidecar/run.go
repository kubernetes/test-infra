/*
Copyright 2018 The Kubernetes Authors.

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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/entrypoint"
	"k8s.io/test-infra/prow/pod-utils/downwardapi"
	"k8s.io/test-infra/prow/pod-utils/gcs"
	"k8s.io/test-infra/prow/pod-utils/wrapper"
)

func nameEntry(idx int, opt wrapper.Options) string {
	return fmt.Sprintf("entry %d: %s", idx, strings.Join(opt.Args, " "))
}

func wait(ctx context.Context, entries []wrapper.Options) (bool, bool, int) {
	passed := true
	var aborted bool
	var failures int

	for i, opt := range entries {
		returnCode, err := wrapper.WaitForMarker(ctx, opt.MarkerFile)
		passed = passed && err == nil && returnCode == 0
		aborted = aborted || returnCode == entrypoint.AbortedErrorCode
		if returnCode != 0 && returnCode != entrypoint.PreviousErrorCode {
			failures++
		}
		if err != nil {
			logrus.Errorf("Error waiting for marker for %s: %v.", nameEntry(i, opt), err)
		}
	}
	return passed, aborted, failures
}

// Run will watch for the process being wrapped to exit
// and then post the status of that process and any artifacts
// to cloud storage.
func (o Options) Run(ctx context.Context) (int, error) {
	spec, err := downwardapi.ResolveSpecFromEnv()
	if err != nil {
		return 0, fmt.Errorf("could not resolve job spec: %v", err)
	}

	if o.DeprecatedWrapperOptions != nil {
		// This only fires if the prowjob controller and sidecar are at different commits
		logrus.Warnf("Using deprecated wrapper_options instead of entries. Please update prow/pod-utils/decorate before June 2019")
	}
	entries := o.entries()

	// If we are being asked to terminate by the kubelet but we have
	// NOT seen the test process exit cleanly, we need a to start
	// uploading artifacts to GCS immediately. If we notice the process
	// exit while doing this best-effort upload, we can race with the
	// second upload but we can tolerate this as we'd rather get SOME
	// data into GCS than attempt to cancel these uploads and get none.
	interrupt := make(chan os.Signal)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)
	go func() {
		s := <-interrupt
		logrus.Errorf("Received an interrupt: %s.", s)
		o.doUpload(spec, false, true, true, entries)
	}()

	passed, aborted, failures := wait(ctx, entries)
	// If we are being asked to terminate by the kubelet but we have
	// seen the test process exit cleanly, we need a chance to upload
	// artifacts to GCS. The only valid way for this program to exit
	// after a SIGINT or SIGTERM in this situation is to finish
	// uploading, so we ignore the signals.
	signal.Ignore(os.Interrupt, syscall.SIGTERM)

	return failures, o.doUpload(spec, passed, aborted, false, entries)
}

const errorKey = "sidecar-errors"

func start(part string) string {
	return fmt.Sprintf("\n==== start of %s log ====\n", part)
}

func logReader(entries []wrapper.Options) io.Reader {
	var readers []io.Reader
	for i, opt := range entries {
		ent := nameEntry(i, opt)
		if len(entries) > 1 {
			readers = append(readers, strings.NewReader(start(ent)))
		}
		log, err := os.Open(opt.ProcessLog)
		if err != nil {
			logrus.WithError(err).Errorf("Failed to open %s", opt.ProcessLog)
			readers = append(readers, strings.NewReader(fmt.Sprintf("Failed to open %s: %v\n", opt.ProcessLog, err)))
		} else {
			readers = append(readers, log)
		}
	}
	return io.MultiReader(readers...)
}

func combineMetadata(entries []wrapper.Options) map[string]interface{} {
	errors := map[string]error{}
	metadata := map[string]interface{}{}
	for i, opt := range entries {
		ent := nameEntry(i, opt)
		metadataFile := opt.MetadataFile
		if _, err := os.Stat(metadataFile); err != nil {
			if !os.IsNotExist(err) {
				logrus.WithError(err).Errorf("Failed to stat %s", metadataFile)
				errors[ent] = err
			}
			continue
		}
		metadataRaw, err := ioutil.ReadFile(metadataFile)
		if err != nil {
			logrus.WithError(err).Errorf("cannot read %s", metadataFile)
			errors[ent] = err
			continue
		}

		piece := map[string]interface{}{}
		if err := json.Unmarshal(metadataRaw, &piece); err != nil {
			logrus.WithError(err).Errorf("Failed to unmarshal %s", metadataFile)
			errors[ent] = err
			continue
		}

		for k, v := range piece {
			metadata[k] = v // TODO(fejta): consider deeper merge
		}
	}
	if len(errors) > 0 {
		metadata[errorKey] = errors
	}
	return metadata
}

func (o Options) doUpload(spec *downwardapi.JobSpec, passed, aborted, interrupt bool, entries []wrapper.Options) error {
	buildLog := logReader(entries)
	if interrupt {
		buildLog = io.MultiReader(buildLog, strings.NewReader("The pod was interrupted by the kubelet and the test container(s) did not write to marker file(s)."))
		// If the test containers do write to their marker files before the pod is
		// terminated then this will be overwritten by the resulting extra upload.
	}

	uploadTargets := map[string]gcs.UploadFunc{
		"build-log.txt": gcs.DataUpload(buildLog),
	}

	var result string
	switch {
	case passed:
		result = "SUCCESS"
	case aborted:
		result = "ABORTED"
	default:
		result = "FAILURE"
	}

	now := time.Now().Unix()
	finished := gcs.Finished{
		Timestamp: &now,
		Passed:    &passed,
		Result:    result,
		Metadata:  combineMetadata(entries),
		// TODO(fejta): JobVersion,
	}

	// TODO(fejta): move to initupload and Started.Repos, RepoVersion
	finished.Revision = downwardapi.GetRevisionFromSpec(spec)

	finishedData, err := json.Marshal(&finished)
	if err != nil {
		logrus.WithError(err).Warn("Could not marshal finishing data")
	} else {
		uploadTargets["finished.json"] = gcs.DataUpload(bytes.NewBuffer(finishedData))
	}

	if err := o.GcsOptions.Run(spec, uploadTargets); err != nil {
		return fmt.Errorf("failed to upload to GCS: %v", err)
	}

	return nil
}
