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
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/version"

	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/pod-utils/options"
	"k8s.io/test-infra/prow/sidecar"
)

func main() {
	logrusutil.ComponentInit()
	logrus.SetLevel(logrus.DebugLevel)

	o := sidecar.NewOptions()
	if err := options.Load(o); err != nil {
		logrus.Fatalf("Could not resolve options: %v", err)
	}

	if err := o.Validate(); err != nil {
		logrus.Fatalf("Invalid options: %v", err)
	}

	// also write our logs to a file so that we can expose our output somehow,
	// even if it will be truncated during upload
	tempDir, err := os.MkdirTemp("", version.Name)
	if err != nil {
		logrus.WithError(err).Fatalf("Failed to create log dir.")
	}
	o.GcsOptions.Items = append(o.GcsOptions.Items, tempDir)
	logFile, err := os.Create(filepath.Join(tempDir, version.Name + ".log"))
	if err != nil {
		logrus.WithError(err).Fatalf("Failed to create log file.")
	}
	logrus.AddHook(&formattingHook{
		formatter: logrus.StandardLogger().Formatter,
		writer:    logFile,
	})

	failures, err := o.Run(context.Background())
	if err != nil {
		logrus.WithError(err).Error("Failed to report job status")
	}
	if failures > 0 && o.EntryError {
		logrus.Fatalf("%d containers failed", failures)
	}
}

type formattingHook struct {
	formatter logrus.Formatter
	writer    io.Writer
}

func (hook *formattingHook) Fire(entry *logrus.Entry) error {
	line, err := hook.formatter.Format(entry)
	if err != nil {
		return err
	}
	_, err = hook.writer.Write(line)
	return err
}

func (hook *formattingHook) Levels() []logrus.Level {
	return logrus.AllLevels
}
