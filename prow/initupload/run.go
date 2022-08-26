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

package initupload

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/GoogleCloudPlatform/testgrid/metadata"
	prowv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/pod-utils/clone"
	"k8s.io/test-infra/prow/pod-utils/downwardapi"
	"k8s.io/test-infra/prow/pod-utils/gcs"
)

// Run will start the initupload job to upload the artifacts, logs and clone status.
func (o Options) Run() error {
	spec, err := downwardapi.ResolveSpecFromEnv()
	if err != nil {
		return fmt.Errorf("could not resolve job spec: %w", err)
	}

	uploadTargets := map[string]gcs.UploadFunc{}

	var failed bool
	var cloneRecords []clone.Record
	if o.Log != "" {
		if failed, cloneRecords, err = processCloneLog(o.Log, uploadTargets); err != nil {
			return err
		}
	}

	started := downwardapi.SpecToStarted(spec, cloneRecords)

	startedData, err := json.Marshal(&started)
	if err != nil {
		return fmt.Errorf("could not marshal starting data: %w", err)
	}

	uploadTargets[prowv1.StartedStatusFile] = gcs.DataUpload(bytes.NewReader(startedData))

	ctx := context.Background()
	if err := o.Options.Run(ctx, spec, uploadTargets); err != nil {
		return fmt.Errorf("failed to upload to blob storage: %w", err)
	}

	if failed {
		return errors.New("cloning the appropriate refs failed")
	}

	return nil
}

// processCloneLog checks if clone operation succeeded or failed for a ref
// and upload clone logs as build log upon failures.
// returns: bool - clone status
//          []Record - containing final SHA on successful clones
//          error - when unexpected file operation happens
func processCloneLog(logfile string, uploadTargets map[string]gcs.UploadFunc) (bool, []clone.Record, error) {
	var cloneRecords []clone.Record
	data, err := ioutil.ReadFile(logfile)
	if err != nil {
		return true, cloneRecords, fmt.Errorf("could not read clone log: %w", err)
	}
	if err = json.Unmarshal(data, &cloneRecords); err != nil {
		return true, cloneRecords, fmt.Errorf("could not unmarshal clone records: %w", err)
	}
	// Do not read from cloneLog directly. Instead create multiple readers from cloneLog so it can
	// be uploaded to both clone-log.txt and build-log.txt on failure.
	cloneLog := bytes.Buffer{}
	var failed bool
	for _, record := range cloneRecords {
		cloneLog.WriteString(clone.FormatRecord(record))
		failed = failed || record.Failed

	}
	uploadTargets["clone-log.txt"] = gcs.DataUpload(bytes.NewReader(cloneLog.Bytes()))
	uploadTargets[prowv1.CloneRecordFile] = gcs.FileUpload(logfile)

	if failed {
		uploadTargets["build-log.txt"] = gcs.DataUpload(bytes.NewReader(cloneLog.Bytes()))

		passed := !failed
		now := time.Now().Unix()
		finished := metadata.Finished{
			Timestamp: &now,
			Passed:    &passed,
			Result:    "FAILURE",
		}
		finishedData, err := json.Marshal(&finished)
		if err != nil {
			return true, cloneRecords, fmt.Errorf("could not marshal finishing data: %w", err)
		}
		uploadTargets[prowv1.FinishedStatusFile] = gcs.DataUpload(bytes.NewReader(finishedData))
	}
	return failed, cloneRecords, nil
}
