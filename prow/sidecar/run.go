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

	prowv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/entrypoint"
	"k8s.io/test-infra/prow/pod-utils/downwardapi"
	"k8s.io/test-infra/prow/pod-utils/gcs"
	"k8s.io/test-infra/prow/pod-utils/wrapper"
)

func nameEntry(idx int, opt wrapper.Options) string {
	return fmt.Sprintf("entry %d: %s", idx, strings.Join(opt.Args, " "))
}

func wait(ctx context.Context, entries []wrapper.Options) (bool, bool, int) {

	var paths []string

	for _, opt := range entries {
		paths = append(paths, opt.MarkerFile)
	}

	results := wrapper.WaitForMarkers(ctx, paths...)

	passed := true
	var aborted bool
	var failures int

	for _, res := range results {
		passed = passed && res.Err == nil && res.ReturnCode == 0
		aborted = aborted || res.ReturnCode == entrypoint.AbortedErrorCode
		if res.ReturnCode != 0 && res.ReturnCode != entrypoint.PreviousErrorCode {
			failures++
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

	ctx, cancel := context.WithCancel(ctx)

	interrupt := make(chan os.Signal)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)
	go func() {
		select {
		case s := <-interrupt:
			if o.IgnoreInterrupts {
				logrus.Warnf("Received an interrupt: %s, ignoring...", s)
			} else {
				// If we are being asked to terminate by the kubelet but we have
				// NOT seen the test process exit cleanly, we need a to start
				// uploading artifacts to GCS immediately. If we notice the process
				// exit while doing this best-effort upload, we can race with the
				// second upload but we can tolerate this as we'd rather get SOME
				// data into GCS than attempt to cancel these uploads and get none.
				logrus.Errorf("Received an interrupt: %s, cancelling...", s)
				cancel()
			}
		case <-ctx.Done():
		}
	}()

	if o.DeprecatedWrapperOptions != nil {
		// This only fires if the prowjob controller and sidecar are at different commits
		logrus.Warnf("Using deprecated wrapper_options instead of entries. Please update prow/pod-utils/decorate before June 2019")
	}
	entries := o.entries()
	passed, aborted, failures := wait(ctx, entries)

	cancel()
	// If we are being asked to terminate by the kubelet but we have
	// seen the test process exit cleanly, we need a chance to upload
	// artifacts to GCS. The only valid way for this program to exit
	// after a SIGINT or SIGTERM in this situation is to finish
	// uploading, so we ignore the signals.
	signal.Ignore(os.Interrupt, syscall.SIGTERM)

	if len(o.SecretDirectories) > 0 {
		if err := o.censor(); err != nil {
			logrus.Warnf("Failed to censor data: %v", err)
		}
	}
	buildLogs := logReaders(entries)
	metadata := combineMetadata(entries)
	return failures, o.doUpload(spec, passed, aborted, metadata, buildLogs)
}

const errorKey = "sidecar-errors"

func logReaders(entries []wrapper.Options) map[string]io.Reader {
	readers := make(map[string]io.Reader)
	for _, opt := range entries {
		buildLog := "build-log.txt"
		if len(entries) > 1 {
			buildLog = fmt.Sprintf("%s-build-log.txt", opt.ContainerName)
		}
		log, err := os.Open(opt.ProcessLog)
		if err != nil {
			logrus.WithError(err).Errorf("Failed to open %s", opt.ProcessLog)
			readers[buildLog] = strings.NewReader(fmt.Sprintf("Failed to open %s: %v\n", opt.ProcessLog, err))
		} else {
			readers[buildLog] = log
		}
	}
	return readers
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

func (o Options) doUpload(spec *downwardapi.JobSpec, passed, aborted bool, metadata map[string]interface{}, logReaders map[string]io.Reader) error {
	uploadTargets := make(map[string]gcs.UploadFunc)

	for logName, reader := range logReaders {
		uploadTargets[logName] = gcs.DataUpload(reader)
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
		Metadata:  metadata,
		// TODO(fejta): JobVersion,
	}

	// TODO(fejta): move to initupload and Started.Repos, RepoVersion
	finished.DeprecatedRevision = downwardapi.GetRevisionFromSpec(spec)

	finishedData, err := json.Marshal(&finished)
	if err != nil {
		logrus.WithError(err).Warn("Could not marshal finishing data")
	} else {
		uploadTargets[prowv1.FinishedStatusFile] = gcs.DataUpload(bytes.NewBuffer(finishedData))
	}

	if err := o.GcsOptions.Run(spec, uploadTargets); err != nil {
		return fmt.Errorf("failed to upload to GCS: %v", err)
	}

	return nil
}
