/*
Copyright 2016 The Kubernetes Authors.

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
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Sirupsen/logrus"

	"k8s.io/test-infra/prow/kube"
)

const (
	namespace = "default"
	maxJobs   = 500
)

var ja *JobAgent

func main() {
	logrus.SetFormatter(&logrus.JSONFormatter{})

	kc, err := kube.NewClientInCluster(namespace)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting client.")
	}

	ja = &JobAgent{
		kc: kc,
	}
	ja.Start()

	http.Handle("/", http.FileServer(http.Dir("/static")))
	http.HandleFunc("/data.js", func(w http.ResponseWriter, r *http.Request) {
		jobs := ja.Jobs()
		if len(jobs) > maxJobs {
			jobs = jobs[:maxJobs]
		}
		jd, err := json.Marshal(jobs)
		if err != nil {
			logrus.WithError(err).Error("Error marshaling jobs.")
			jd = []byte("[]")
		}
		v := "allBuilds"
		if nv := r.URL.Query().Get("var"); nv != "" {
			v = nv
		}
		w.Header().Set("Cache-Control", "no-cache")
		fmt.Fprintf(w, "var %s = %s;", v, string(jd))
	})
	logrus.WithError(http.ListenAndServe(":http", nil)).Fatal("ListenAndServe returned.")
}
