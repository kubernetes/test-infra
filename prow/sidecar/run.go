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
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/pod-utils/gcs"
)

// Run will watch for the process being wrapped to exit
// and then post the status of that process and any artifacts
// to cloud storage.
func (o Options) Run() error {
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

	passed := false
	returnCodeData, err := ioutil.ReadFile(o.WrapperOptions.MarkerFile)
	if err != nil {
		logrus.WithError(err).Warn("Could not read return code from marker file")
	} else {
		returnCode, err := strconv.Atoi(strings.TrimSpace(string(returnCodeData)))
		if err != nil {
			logrus.WithError(err).Warn("Failed to parse process return code")
		}
		passed = returnCode == 0 && err == nil
	}

	uploadTargets := map[string]gcs.UploadFunc{
		"build-log.txt": gcs.FileUpload(o.WrapperOptions.ProcessLog),
	}

	finished := struct {
		Timestamp int64 `json:"timestamp"`
		Passed    bool  `json:"passed"`
	}{
		Timestamp: time.Now().Unix(),
		Passed:    passed,
	}
	finishedData, err := json.Marshal(&finished)
	if err != nil {
		logrus.WithError(err).Warn("Could not marshal finishing data")
	} else {
		uploadTargets["finished.json"] = gcs.DataUpload(bytes.NewBuffer(finishedData))
	}

	if err := o.GcsOptions.Run(uploadTargets); err != nil {
		return fmt.Errorf("failed to upload to GCS: %v", err)
	}

	return nil
}
