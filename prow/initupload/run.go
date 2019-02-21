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
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"strconv"
	"time"

	"k8s.io/test-infra/prow/pod-utils/clone"
	"k8s.io/test-infra/prow/pod-utils/downwardapi"
	"k8s.io/test-infra/prow/pod-utils/gcs"
)

// specToStarted translate a jobspec into a started struct
// optionally overwrite RepoVersion with provided mainRefSHA
func specToStarted(spec *downwardapi.JobSpec, mainRefSHA string) gcs.Started {
	started := gcs.Started{
		Timestamp:   time.Now().Unix(),
		RepoVersion: downwardapi.GetRevisionFromSpec(spec),
	}

	if mainRefSHA != "" {
		started.RepoVersion = mainRefSHA
	}

	// TODO(fejta): VM name

	if spec.Refs != nil && len(spec.Refs.Pulls) > 0 {
		started.Pull = strconv.Itoa(spec.Refs.Pulls[0].Number)
	}

	started.Repos = map[string]string{}

	if spec.Refs != nil {
		started.Repos[spec.Refs.Org+"/"+spec.Refs.Repo] = spec.Refs.String()
	}
	for _, ref := range spec.ExtraRefs {
		started.Repos[ref.Org+"/"+ref.Repo] = ref.String()
	}

	return started
}

// Run will start the initupload job to upload the artifacts, logs and clone status.
func (o Options) Run() error {
	spec, err := downwardapi.ResolveSpecFromEnv()
	if err != nil {
		return fmt.Errorf("could not resolve job spec: %v", err)
	}

	uploadTargets := map[string]gcs.UploadFunc{}

	var failed bool
	var mainRefSHA string
	if o.Log != "" {
		if failed, mainRefSHA, err = processCloneLog(o.Log, uploadTargets); err != nil {
			return err
		}
	}

	started := specToStarted(spec, mainRefSHA)

	startedData, err := json.Marshal(&started)
	if err != nil {
		return fmt.Errorf("could not marshal starting data: %v", err)
	}

	uploadTargets["started.json"] = gcs.DataUpload(bytes.NewReader(startedData))

	if err := o.Options.Run(spec, uploadTargets); err != nil {
		return fmt.Errorf("failed to upload to GCS: %v", err)
	}

	if failed {
		return errors.New("cloning the appropriate refs failed")
	}

	return nil
}

// processCloneLog checks if clone operation successed or failed for a ref
// and upload clone logs as build log upon failures.
// returns: bool - clone status
//          string - final main ref SHA on a successful clone
//          error - when unexpected file operation happens
func processCloneLog(logfile string, uploadTargets map[string]gcs.UploadFunc) (bool, string, error) {
	var cloneRecords []clone.Record
	data, err := ioutil.ReadFile(logfile)
	if err != nil {
		return true, "", fmt.Errorf("could not read clone log: %v", err)
	}
	if err = json.Unmarshal(data, &cloneRecords); err != nil {
		return true, "", fmt.Errorf("could not unmarshal clone records: %v", err)
	}
	// Do not read from cloneLog directly. Instead create multiple readers from cloneLog so it can
	// be uploaded to both clone-log.txt and build-log.txt on failure.
	cloneLog := bytes.Buffer{}
	var failed bool
	var mainRefSHA string
	for idx, record := range cloneRecords {
		cloneLog.WriteString(clone.FormatRecord(record))
		failed = failed || record.Failed
		// fill in mainRefSHA with FinalSHA from the first record
		if idx == 0 {
			mainRefSHA = record.FinalSHA
		}

	}
	uploadTargets["clone-log.txt"] = gcs.DataUpload(bytes.NewReader(cloneLog.Bytes()))
	uploadTargets["clone-records.json"] = gcs.FileUpload(logfile)

	if failed {
		uploadTargets["build-log.txt"] = gcs.DataUpload(bytes.NewReader(cloneLog.Bytes()))

		passed := !failed
		now := time.Now().Unix()
		finished := gcs.Finished{
			Timestamp: &now,
			Passed:    &passed,
			Result:    "FAILURE",
		}
		finishedData, err := json.Marshal(&finished)
		if err != nil {
			return true, mainRefSHA, fmt.Errorf("could not marshal finishing data: %v", err)
		}
		uploadTargets["finished.json"] = gcs.DataUpload(bytes.NewReader(finishedData))
	}
	return failed, mainRefSHA, nil
}
