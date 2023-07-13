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
	"fmt"
	"os"
	"path"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/entrypoint"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/pod-utils/options"
)

// copy copies entrypoint binary from source to destination. This is because
// entrypoint image operates in two different modes:
//  1. entrypoint container: copy the binary to shared mount drive `/tools`
//  2. test container(s): use `/tools/entrypoint` as entrypoint, for collecting
//     logs and artifacts.
func copy(src, dst string) error {
	logrus.Infof("src is %s", src)
	// Get file info so that the mode can be used for copying
	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("read info '%s': %w", src, err)
	}
	body, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read file '%s': %w", src, err)
	}
	// Create dir if not exist
	dstDir := path.Dir(dst)
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return fmt.Errorf("create dir '%s': %w", dstDir, err)
	}
	if err := os.WriteFile(dst, body, info.Mode()); err != nil {
		return fmt.Errorf("write file '%s': %w", dst, err)
	}
	return nil
}

func main() {
	logrusutil.ComponentInit()

	o := entrypoint.NewOptions()
	if err := options.Load(o); err != nil {
		logrus.Fatalf("Could not resolve options: %v", err)
	}

	if o.CopyModeOnly {
		if err := copy(os.Args[0], o.CopyDst); err != nil {
			logrus.WithError(err).Fatal("Failed running in copy mode, this is a prow bug.")
		}
		os.Exit(0)
	}

	if err := o.Validate(); err != nil {
		logrus.Fatalf("Invalid options: %v", err)
	}

	os.Exit(o.Run())
}
