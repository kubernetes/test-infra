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

	"github.com/NYTimes/gziphandler"
	"github.com/Sirupsen/logrus"

	"k8s.io/test-infra/prow/kube"
)

const (
	namespace = "default"
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

	http.Handle("/", gziphandler.GzipHandler(http.FileServer(http.Dir("/static"))))
	http.Handle("/data.js", gziphandler.GzipHandler(http.HandlerFunc(handleData)))

	logrus.WithError(http.ListenAndServe(":http", nil)).Fatal("ListenAndServe returned.")
}

func handleData(w http.ResponseWriter, r *http.Request) {
	jobs := ja.Jobs()
	jd, err := json.Marshal(jobs)
	if err != nil {
		logrus.WithError(err).Error("Error marshaling jobs.")
		jd = []byte("[]")
	}
	// If we have a "var" query, then write out "var value = {...};".
	// Otherwise, just write out the JSON.
	if v := r.URL.Query().Get("var"); v != "" {
		fmt.Fprintf(w, "var %s = %s;", v, string(jd))
	} else {
		fmt.Fprintf(w, string(jd))
	}
	w.Header().Set("Cache-Control", "no-cache")
}
