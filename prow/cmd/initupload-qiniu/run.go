package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strconv"
	"time"

	"k8s.io/test-infra/prow/pod-utils/clone"
	"k8s.io/test-infra/prow/pod-utils/downwardapi"
	"k8s.io/test-infra/prow/pod-utils/gcs"
	"k8s.io/test-infra/prow/qiniu"
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

// processCloneLog checks if clone operation successed or failed for a ref
// and upload clone logs as build log upon failures.
// returns: bool - clone status
//          string - final main ref SHA on a successful clone
//          error - when unexpected file operation happens
func processCloneLog(logfile string, uploadTargets map[string]qiniu.UploadFunc, prefix string) (bool, string, error) {
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
	key := prefix + "/clone-log.txt"
	uploadTargets[key] = qiniu.DataUpload(key, bytes.NewReader(cloneLog.Bytes()))
	key = prefix + "/clone-records.json"
	uploadTargets[key] = qiniu.FileUpload(key, logfile)

	if failed {
		key = prefix + "/build-log.txt"
		uploadTargets[key] = qiniu.DataUpload(key, bytes.NewReader(cloneLog.Bytes()))

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
		key = prefix + "/finished.json"
		uploadTargets[key] = qiniu.DataUpload(key, bytes.NewReader(finishedData))
	}
	return failed, mainRefSHA, nil
}
