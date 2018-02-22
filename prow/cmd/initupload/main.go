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

// initupload parses the logs from the clonerefs
// container and determines if that container was
// successful or not. Using that information, this
// container uploads started.json and if necessary
// finished.json as well.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"io/ioutil"
	"os"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/pod-utils/clone"
	"k8s.io/test-infra/prow/pod-utils/gcs"
)

type options struct {
	cloneLog string

	gcsOptions *gcs.Options
}

func (o *options) Validate() error {
	if o.cloneLog == "" {
		return errors.New("required flag --clone-logs was unset")
	}

	return o.gcsOptions.Validate()
}

func gatherOptions() options {
	o := options{}
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	fs.StringVar(&o.cloneLog, "clone-log", "", "path to the output file for the cloning step")
	o.gcsOptions = gcs.BindOptions(fs)
	fs.Parse(os.Args[1:])
	o.gcsOptions.Complete(fs.Args())
	return o
}

func main() {
	o := gatherOptions()
	if err := o.Validate(); err != nil {
		logrus.Fatalf("Invalid options: %v", err)
	}

	var cloneRecords []clone.Record
	data, err := ioutil.ReadFile(o.cloneLog)
	if err != nil {
		logrus.WithError(err).Fatal("Could not read clone log")
	}
	if err = json.Unmarshal(data, &cloneRecords); err != nil {
		logrus.WithError(err).Fatal("Could not unmarshal clone records")
	}

	failed := false
	buildLog := bytes.Buffer{}
	for _, record := range cloneRecords {
		buildLog.WriteString(clone.FormatRecord(record))
		failed = failed || record.Failed
	}

	uploadTargets := map[string]gcs.UploadFunc{
		"clone-log.txt":      gcs.DataUpload(&buildLog),
		"clone-records.json": gcs.FileUpload(o.cloneLog),
	}

	started := struct {
		Timestamp int64 `json:"timestamp"`
	}{
		Timestamp: time.Now().Unix(),
	}
	startedData, err := json.Marshal(&started)
	if err != nil {
		logrus.WithError(err).Fatal("Could not marshal starting data")
	} else {
		uploadTargets["started.json"] = gcs.DataUpload(bytes.NewBuffer(startedData))
	}

	if failed {
		finished := struct {
			Timestamp int64  `json:"timestamp"`
			Passed    bool   `json:"passed"`
			Result    string `json:"result"`
		}{
			Timestamp: time.Now().Unix(),
			Passed:    false,
			Result:    "FAILURE",
		}
		finishedData, err := json.Marshal(&finished)
		if err != nil {
			logrus.WithError(err).Fatal("Could not marshal finishing data")
		} else {
			uploadTargets["build-log.txt"] = gcs.DataUpload(&buildLog)
			uploadTargets["finished.json"] = gcs.DataUpload(bytes.NewBuffer(finishedData))
		}
	}

	if err := o.gcsOptions.Run(uploadTargets); err != nil {
		logrus.WithError(err).Fatal("Failed to upload to GCS")
	}

	if failed {
		logrus.Fatal("Cloning the appropriate refs failed.")
	}
}
