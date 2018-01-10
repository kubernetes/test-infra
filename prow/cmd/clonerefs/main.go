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
	"flag"
	"fmt"
	"io/ioutil"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pjutil"
	"k8s.io/test-infra/prow/pod-utils/clone"
)

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

var (
	srcRoot = flag.String("src-root", "", "Where to root source checkouts")
	log     = flag.String("log", "", "Where to write logs")
)

func main() {
	var gitRefs gitRefs
	flag.Var(&gitRefs, "repo", "Mapping of Git URI to refs to check out, can be provided more than once")
	flag.Parse()

	if *srcRoot == "" {
		logrus.Fatal("No source root specified")
	}

	if *log == "" {
		logrus.Fatal("No log file specified")
	}

	jobRefs, err := pjutil.ResolveSpecFromEnv()
	if err != nil {
		logrus.WithError(err).Fatal("Could not determine job refs")
	}

	results := []clone.Record{
		clone.Run(jobRefs.Refs, *srcRoot),
	}
	for _, gitRef := range gitRefs.gitRefs {
		results = append(results, clone.Run(gitRef, *srcRoot))
	}

	logData, err := json.Marshal(results)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to marshal clone records")
	} else {
		if err := ioutil.WriteFile(*log, logData, 0755); err != nil {
			logrus.WithError(err).Fatal("Failed to write clone records")
		}
	}
}
