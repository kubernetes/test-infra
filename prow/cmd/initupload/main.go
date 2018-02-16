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
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"path"
	"time"

	"cloud.google.com/go/storage"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/option"

	"k8s.io/test-infra/prow/pjutil"
	"k8s.io/test-infra/prow/pod-utils/clone"
	"k8s.io/test-infra/prow/pod-utils/gcs"
)

type options struct {
	cloneLog string

	subDir string

	pathStrategy string
	defaultOrg   string
	defaultRepo  string

	gcsBucket          string
	gceCredentialsFile string
	dryRun             bool
}

func (o *options) Validate() error {
	if !o.dryRun {
		if o.gcsBucket == "" {
			return errors.New("GCS upload was requested but required flag --gcs-bucket was unset")
		}

		if o.gceCredentialsFile == "" {
			return errors.New("GCS upload was requested but required flag --gcs-credentials-file was unset")
		}
	}

	if o.cloneLog == "" {
		return errors.New("required flag --clone-logs was unset")
	}

	strategy := pathStrategyType(o.pathStrategy)
	if strategy != pathStrategyLegacy && strategy != pathStrategyExplicit && strategy != pathStrategySingle {
		return fmt.Errorf("path strategy must be one of %q, %q, or %q", pathStrategyLegacy, pathStrategyExplicit, pathStrategySingle)
	}

	if strategy != pathStrategyExplicit && (o.defaultOrg == "" || o.defaultRepo == "") {
		return fmt.Errorf("default org and repo must be provided for strategy %q", strategy)
	}

	return nil
}

func gatherOptions() options {
	o := options{}
	flag.StringVar(&o.cloneLog, "clone-log", "", "path to the output file for the cloning step")

	flag.StringVar(&o.subDir, "sub-dir", "", "Optional sub-directory of the job's path to which artifacts are uploaded")

	flag.StringVar(&o.pathStrategy, "path-strategy", pathStrategyExplicit, "how to encode org and repo into GCS paths")
	flag.StringVar(&o.defaultOrg, "default-org", "", "optional default org for GCS path encoding")
	flag.StringVar(&o.defaultRepo, "default-repo", "", "optional default repo for GCS path encoding")

	flag.StringVar(&o.gcsBucket, "gcs-bucket", "", "GCS bucket to upload into")
	flag.StringVar(&o.gceCredentialsFile, "gcs-credentials-file", "", "file where Google Cloud authentication credentials are stored")
	flag.BoolVar(&o.dryRun, "dry-run", true, "do not interact with GCS")
	flag.Parse()
	return o
}

type pathStrategyType string

const (
	pathStrategyLegacy   pathStrategyType = "legacy"
	pathStrategyExplicit                  = "explicit"
	pathStrategySingle                    = "single"
)

func main() {
	o := gatherOptions()
	if err := o.Validate(); err != nil {
		logrus.Fatalf("Invalid options: %v", err)
	}

	var builder gcs.RepoPathBuilder
	switch pathStrategyType(o.pathStrategy) {
	case pathStrategyExplicit:
		builder = gcs.NewExplicitRepoPathBuilder()
	case pathStrategyLegacy:
		builder = gcs.NewLegacyRepoPathBuilder(o.defaultOrg, o.defaultRepo)
	case pathStrategySingle:
		builder = gcs.NewSingleDefaultRepoPathBuilder(o.defaultOrg, o.defaultRepo)
	}

	spec, err := pjutil.ResolveSpecFromEnv()
	if err != nil {
		logrus.WithError(err).Fatal("Could not resolve job spec")
	}

	gcsPath := gcs.PathForSpec(spec, builder)
	if o.subDir != "" {
		gcsPath = path.Join(gcsPath, o.subDir)
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
		path.Join(gcsPath, "clone-log.txt"):      gcs.DataUpload(&buildLog),
		path.Join(gcsPath, "clone-records.json"): gcs.FileUpload(o.cloneLog),
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
		uploadTargets[path.Join(gcsPath, "started.json")] = gcs.DataUpload(bytes.NewBuffer(startedData))
	}

	if failed {
		finished := struct {
			Timestamp int64 `json:"timestamp"`
			Passed    bool  `json:"passed"`
		}{
			Timestamp: time.Now().Unix(),
			Passed:    false,
		}
		finishedData, err := json.Marshal(&finished)
		if err != nil {
			logrus.WithError(err).Fatal("Could not marshal finishing data")
		} else {
			uploadTargets[path.Join(gcsPath, "build-log.txt")] = gcs.DataUpload(&buildLog)
			uploadTargets[path.Join(gcsPath, "finished.json")] = gcs.DataUpload(bytes.NewBuffer(finishedData))
		}
	}

	if !o.dryRun {
		ctx := context.Background()
		gcsClient, err := storage.NewClient(ctx, option.WithCredentialsFile(o.gceCredentialsFile))
		if err != nil {
			logrus.WithError(err).Fatal("Could not connect to GCS")
		}

		if err := gcs.Upload(gcsClient.Bucket(o.gcsBucket), uploadTargets); err != nil {
			logrus.WithError(err).Fatal("Failed to upload to GCS")
		}
	}

	if failed {
		logrus.Fatal("Cloning the appropriate refs failed.")
	}
}
