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
	"flag"
	"os"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/ghhook"
	"k8s.io/test-infra/prow/logrusutil"
)

func main() {
	logrusutil.ComponentInit()

	set := flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	o, err := ghhook.GetOptions(set, os.Args[1:])
	if err != nil {
		logrus.WithError(err).Fatal("Error parsing the given args.")
	}

	if err := o.HandleWebhookConfigChange(); err != nil {
		logrus.WithError(err).Fatal("Error handling the webhook config change.")
	}
}
