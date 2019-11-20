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

// initupload parses the logs from the clonerefs
// container and determines if that container was
// successful or not. Using that information, this
// container uploads started.json and if necessary
// finished.json as well.
package main

import (
	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/initupload"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/pod-utils/options"
)

func main() {
	logrusutil.ComponentInit()

	o := initupload.NewOptions()
	if err := options.Load(o); err != nil {
		logrus.Fatalf("Could not resolve options: %v", err)
	}

	if err := o.Validate(); err != nil {
		logrus.Fatalf("Invalid options: %v", err)
	}

	if err := o.Run(); err != nil {
		logrus.WithError(err).Fatal("Failed to initialize job")
	}
}
