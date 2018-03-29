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

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/pod-utils/wrapper"

	"k8s.io/test-infra/prow/pod-utils/gcs"
)

type options struct {
	wrapperOptions *wrapper.Options
	gcsOptions     *gcs.Options
}

func (o *options) Validate() error {
	if err := o.wrapperOptions.Validate(); err != nil {
		return err
	}

	return o.gcsOptions.Validate()
}

func gatherOptions() options {
	o := options{}
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	wrapper.BindOptions(o.wrapperOptions, fs)
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

	logrus.SetFormatter(
		logrusutil.NewDefaultFieldsFormatter(nil, logrus.Fields{"component": "sidecar"}),
	)

	// Only start watching file events if the file doesn't exist
	// If the file exists, it means the main process already completed.
	if _, err := os.Stat(o.wrapperOptions.MarkerFile); os.IsNotExist(err) {
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			logrus.WithError(err).Fatal("Could not begin fsnotify watch")
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
					if event.Name == o.wrapperOptions.MarkerFile && event.Op&fsnotify.Create == fsnotify.Create {
						return
					}
				case err := <-watcher.Errors:
					logrus.WithError(err).Info("Encountered an error during fsnotify watch")
				case <-ticker.C:
					if _, err := os.Stat(o.wrapperOptions.MarkerFile); err == nil {
						return
					}
				}
			}
		}()

		dir := filepath.Dir(o.wrapperOptions.MarkerFile)
		if err := watcher.Add(dir); err != nil {
			logrus.WithError(err).Fatal("Could not add to fsnotify watch")
		}
		group.Wait()
		ticker.Stop()
	}

	passed := false
	returnCodeData, err := ioutil.ReadFile(o.wrapperOptions.MarkerFile)
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
		"build-log.txt": gcs.FileUpload(o.wrapperOptions.ProcessLog),
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

	if err := o.gcsOptions.Run(uploadTargets); err != nil {
		logrus.WithError(err).Fatal("Failed to upload to GCS")
	}
}
