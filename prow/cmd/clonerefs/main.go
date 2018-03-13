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
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/logrusutil"

	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pjutil"
	"k8s.io/test-infra/prow/pod-utils/clone"
)

type options struct {
	srcRoot string
	log     string

	gitUserName  string
	gitUserEmail string

	refs gitRefs
}

func (o *options) Validate() error {
	if o.srcRoot == "" {
		return errors.New("no source root specified")
	}

	if o.log == "" {
		return errors.New("no log file specified")
	}

	seen := map[string]sets.String{}
	for _, ref := range o.refs.gitRefs {
		if _, seenOrg := seen[ref.Org]; seenOrg {
			if seen[ref.Org].Has(ref.Repo) {
				return errors.New("sync config for %s/%s provided more than once")
			}

			seen[ref.Org].Insert(ref.Repo)
		} else {
			seen[ref.Org] = sets.NewString(ref.Repo)
		}
	}

	return nil
}

func gatherOptions() options {
	o := options{}
	flag.StringVar(&o.srcRoot, "src-root", "", "Where to root source checkouts")
	flag.StringVar(&o.log, "log", "", "Where to write logs")
	flag.StringVar(&o.gitUserName, "git-user-name", "ci-robot", "Username to set in git config")
	flag.StringVar(&o.gitUserEmail, "git-user-email", "ci-robot@k8s.io", "Email to set in git config")
	flag.Var(&o.refs, "repo", "Mapping of Git URI to refs to check out, can be provided more than once")
	flag.Parse()
	return o
}

type gitRefs struct {
	gitRefs []kube.Refs
}

func (r *gitRefs) String() string {
	representation := bytes.Buffer{}
	for _, ref := range r.gitRefs {
		fmt.Fprintf(&representation, "%s,%s=%s", ref.Org, ref.Repo, ref.String())
	}
	return representation.String()
}

// Set parses out a kube.Refs from the user string.
// The following example shows all possible fields:
//   org,repo=base-ref:base-sha[,pull-number:pull-sha]...
// For the base ref and every pull number, the SHAs
// are optional and any number of them may be set or
// unset.
func (r *gitRefs) Set(value string) error {
	gitRef, err := clone.ParseRefs(value)
	if err != nil {
		return err
	}
	r.gitRefs = append(r.gitRefs, gitRef)
	return nil
}

func main() {
	o := gatherOptions()
	if err := o.Validate(); err != nil {
		logrus.Fatalf("Invalid options: %v", err)
	}

	logrus.SetFormatter(
		logrusutil.NewDefaultFieldsFormatter(nil, logrus.Fields{"component": "clonerefs"}),
	)

	var results []clone.Record

	jobRefs, err := pjutil.ResolveSpecFromEnv()
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
			results = append(results, clone.Run(jobRefs.Refs, o.srcRoot, o.gitUserName, o.gitUserEmail))
		}
	}

	for _, gitRef := range o.refs.gitRefs {
		results = append(results, clone.Run(gitRef, o.srcRoot, o.gitUserName, o.gitUserEmail))
	}

	logData, err := json.Marshal(results)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to marshal clone records")
	} else {
		if err := ioutil.WriteFile(o.log, logData, 0755); err != nil {
			logrus.WithError(err).Fatal("Failed to write clone records")
		}
	}

	logrus.Info("Finished cloning refs")
}
