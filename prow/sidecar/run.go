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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pod-utils/downwardapi"
	"k8s.io/test-infra/prow/pod-utils/gcs"
)

// Run will watch for the process being wrapped to exit
// and then post the status of that process and any artifacts
// to cloud storage.
func (o Options) Run() error {
	spec, err := downwardapi.ResolveSpecFromEnv()
	if err != nil {
		return fmt.Errorf("could not resolve job spec: %v", err)
	}

	// If we are being asked to terminate by the kubelet but we have
	// NOT seen the test process exit cleanly, we need a to start
	// uploading artifacts to GCS immediately. If we notice the process
	// exit while doing this best-effort upload, we can race with the
	// second upload but we can tolerate this as we'd rather get SOME
	// data into GCS than attempt to cancel these uploads and get none.
	interrupt := make(chan os.Signal)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)
	go func() {
		select {
		case s := <-interrupt:
			logrus.Errorf("Received an interrupt: %s", s)
			o.doUpload(spec, false, true, nil)
		}
	}()

	// Only start watching file events if the file doesn't exist
	// If the file exists, it means the main process already completed.
	if _, err := os.Stat(o.WrapperOptions.MarkerFile); os.IsNotExist(err) {
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			return fmt.Errorf("could not begin fsnotify watch: %v", err)
		}
		defer watcher.Close()

		ticker := time.NewTicker(30 * time.Second)
		group := sync.WaitGroup{}
		group.Add(1)
		go func() {
			defer group.Done()
			for {
				select {
				case event := <-watcher.Events:
					if event.Name == o.WrapperOptions.MarkerFile && event.Op&fsnotify.Create == fsnotify.Create {
						return
					}
				case err := <-watcher.Errors:
					logrus.WithError(err).Info("Encountered an error during fsnotify watch")
				case <-ticker.C:
					if _, err := os.Stat(o.WrapperOptions.MarkerFile); err == nil {
						return
					}
				}
			}
		}()

		dir := filepath.Dir(o.WrapperOptions.MarkerFile)
		if err := watcher.Add(dir); err != nil {
			return fmt.Errorf("could not add to fsnotify watch: %v", err)
		}
		group.Wait()
		ticker.Stop()
	}

	// If we are being asked to terminate by the kubelet but we have
	// seen the test process exit cleanly, we need a chance to upload
	// artifacts to GCS. The only valid way for this program to exit
	// after a SIGINT or SIGTERM in this situation is to finish]
	// uploading, so we ignore the signals.
	signal.Ignore(os.Interrupt, syscall.SIGTERM)

	passed := false
	aborted := false
	returnCodeData, err := ioutil.ReadFile(o.WrapperOptions.MarkerFile)
	if err != nil {
		logrus.WithError(err).Warn("Could not read return code from marker file")
	} else {
		returnCode, err := strconv.Atoi(strings.TrimSpace(string(returnCodeData)))
		if err != nil {
			logrus.WithError(err).Warn("Failed to parse process return code")
		}
		passed = returnCode == 0 && err == nil
		aborted = returnCode == 130
	}

	metadataFile := o.WrapperOptions.MetadataFile
	if _, err := os.Stat(metadataFile); err != nil {
		if !os.IsNotExist(err) {
			logrus.WithError(err).Errorf("Failed to stat %s", metadataFile)
		}
		return o.doUpload(spec, passed, aborted, nil)
	}

	metadataRaw, err := ioutil.ReadFile(metadataFile)
	if err != nil {
		logrus.WithError(err).Errorf("cannot read %s", metadataFile)
		return o.doUpload(spec, passed, aborted, nil)
	}

	metadata := map[string]interface{}{}
	if err := json.Unmarshal(metadataRaw, &metadata); err != nil {
		logrus.WithError(err).Errorf("Failed to unmarshal %s", metadataFile)
		return o.doUpload(spec, passed, aborted, nil)
	}

	return o.doUpload(spec, passed, aborted, metadata)
}

func getRevisionFromRef(refs *kube.Refs) string {
	if len(refs.Pulls) > 0 {
		return refs.Pulls[0].SHA
	}

	if refs.BaseSHA != "" {
		return refs.BaseSHA
	}

	return refs.BaseRef
}

func (o Options) doUpload(spec *downwardapi.JobSpec, passed, aborted bool, metadata map[string]interface{}) error {
	uploadTargets := map[string]gcs.UploadFunc{
		"build-log.txt": gcs.FileUpload(o.WrapperOptions.ProcessLog),
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

	// TODO(krzyzacy): Unify with downstream spyglass definition
	finished := struct {
		Timestamp int64                  `json:"timestamp"`
		Passed    bool                   `json:"passed"`
		Result    string                 `json:"result"`
		Metadata  map[string]interface{} `json:"metadata,omitempty"`
		Revision  string                 `json:"revision,omitempty"`
	}{
		Timestamp: time.Now().Unix(),
		Passed:    passed,
		Result:    result,
		Metadata:  metadata,
	}

	if spec.Refs != nil {
		finished.Revision = getRevisionFromRef(spec.Refs)
	} else if len(spec.ExtraRefs) > 0 {
		finished.Revision = getRevisionFromRef(&spec.ExtraRefs[0])
	}

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
