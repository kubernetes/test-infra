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
	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/clonerefs"
	"k8s.io/test-infra/prow/logrusutil"
<<<<<<< HEAD
	"k8s.io/test-infra/prow/pod-utils/clone"
	"k8s.io/test-infra/prow/pod-utils/downwardapi"
=======
>>>>>>> cbf2bd5... work
)

func main() {
	o, err := clonerefs.ResolveOptions()
	if err != nil {
		logrus.Fatalf("Could not resolve options: %v", err)
	}

	if err := o.Validate(); err != nil {
		logrus.Fatalf("Invalid options: %v", err)
	}

	logrus.SetFormatter(
		logrusutil.NewDefaultFieldsFormatter(nil, logrus.Fields{"component": "clonerefs"}),
	)

<<<<<<< HEAD
	wg := &sync.WaitGroup{}
	output := make(chan clone.Record, len(o.refs.gitRefs)+1)

	jobRefs, err := downwardapi.ResolveSpecFromEnv()
	if err != nil {
		logrus.WithError(err).Warn("Could not determine Prow job refs from environment")
	} else {
		if jobRefs.Type != kube.PeriodicJob {
			// periodic jobs do not configure a set
			// of refs to clone, so we ignore them
			for _, gitRef := range o.refs.gitRefs {
				if gitRef.Org == jobRefs.Refs.Org && gitRef.Repo == jobRefs.Refs.Repo {
					logrus.Fatalf("Clone specification for %s/%s found both in Prow variables and user-provided flags", jobRefs.Refs.Org, jobRefs.Refs.Repo)
				}
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				output <- clone.Run(jobRefs.Refs, o.srcRoot, o.gitUserName, o.gitUserEmail, o.aliases.aliases)
			}()
		}
	}

	wg.Add(len(o.refs.gitRefs))
	for _, gitRef := range o.refs.gitRefs {
		go func(ref kube.Refs) {
			defer wg.Done()
			output <- clone.Run(ref, o.srcRoot, o.gitUserName, o.gitUserEmail, o.aliases.aliases)
		}(gitRef)
	}

	wg.Wait()
	close(output)

	var results []clone.Record
	for record := range output {
		results = append(results, record)
	}

	logData, err := json.Marshal(results)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to marshal clone records")
	} else {
		if err := ioutil.WriteFile(o.log, logData, 0755); err != nil {
			logrus.WithError(err).Fatal("Failed to write clone records")
		}
=======
	if err := o.Run(); err != nil {
		logrus.WithError(err).Fatal("Failed to clone refs")
>>>>>>> cbf2bd5... work
	}

	logrus.Info("Finished cloning refs")
}
