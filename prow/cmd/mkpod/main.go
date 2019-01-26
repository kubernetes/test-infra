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

package main

import (
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"

	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pod-utils/decorate"
)

type options struct {
	prowJobPath string
	buildID     string
}

func (o *options) Validate() error {
	if o.prowJobPath == "" {
		return errors.New("required flag --prow-job was unset")
	}

	return nil
}

func gatherOptions() options {
	o := options{}
	flag.StringVar(&o.prowJobPath, "prow-job", "", "ProwJob to decorate, - for stdin.")
	flag.StringVar(&o.buildID, "build-id", "", "Build ID for the job run.")
	flag.Parse()
	return o
}

func main() {
	o := gatherOptions()
	if err := o.Validate(); err != nil {
		logrus.Fatalf("Invalid options: %v", err)
	}

	var rawJob []byte
	if o.prowJobPath == "-" {
		raw, err := ioutil.ReadAll(os.Stdin)
		if err != nil {
			logrus.WithError(err).Fatal("Could not read ProwJob YAML from stdin.")
		}
		rawJob = raw
	} else {
		raw, err := ioutil.ReadFile(o.prowJobPath)
		if err != nil {
			logrus.WithError(err).Fatal("Could not open ProwJob YAML.")
		}
		rawJob = raw
	}

	var job kube.ProwJob
	if err := yaml.Unmarshal(rawJob, &job); err != nil {
		logrus.WithError(err).Fatal("Could not unmarshal ProwJob YAML.")
	}

	if o.buildID == "" && job.Status.BuildID != "" {
		o.buildID = job.Status.BuildID
	}

	if o.buildID == "" {
		logrus.Warning("No BuildID found in ProwJob status or given with --build-id, GCS interaction will be poor.")
	}

	pod, err := decorate.ProwJobToPod(job, o.buildID)
	if err != nil {
		logrus.WithError(err).Fatal("Could not decorate PodSpec.")
	}

	pod.GetObjectKind().SetGroupVersionKind(v1.SchemeGroupVersion.WithKind("Pod"))
	podYAML, err := yaml.Marshal(pod)
	if err != nil {
		logrus.WithError(err).Fatal("Could not marshal Pod YAML.")
	}
	fmt.Println(string(podYAML))
}
