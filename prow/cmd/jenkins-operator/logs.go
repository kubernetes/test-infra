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
	"fmt"
	"net/http"
	"strings"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/jenkins"
)

func handleLog(jc *jenkins.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET")

		// Needs to be a GET request.
		if r.Method != http.MethodGet {
			http.Error(w, "405 Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Needs to get Jenkins logs.
		if !strings.HasSuffix(r.URL.Path, "consoleText") {
			http.Error(w, "403 Forbidden: Request may only access raw Jenkins logs", http.StatusForbidden)
			return
		}

		log, err := jc.GetSkipMetrics(r.URL.Path)
		if err != nil {
			http.Error(w, fmt.Sprintf("Log not found: %v", err), http.StatusNotFound)
			logrus.WithError(err).Warning(fmt.Sprintf("Cannot get logs from Jenkins (GET %s).", r.URL.Path))
			return
		}

		if _, err = w.Write(log); err != nil {
			logrus.WithError(err).Warning("Error writing log.")
		}
	}
}
