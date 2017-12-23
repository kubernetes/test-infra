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

	"k8s.io/kubernetes/test/e2e/perftype"
)

// Downloader is the interface that gets a data from a predefined source.
type Downloader interface {
	getData() (TestToBuildData, error)
}

// BuildData contains job name and a map from build number to perf data
type BuildData struct {
	Builds  map[string][]perftype.DataItem `json:"builds"`
	Job     string                         `json:"job"`
	Version string                         `json:"version"`
}

// TestToBuildData is a map from test name to BuildData
// TODO(random-liu): Use a more complex data structure if we need to support more test in the future.
type TestToBuildData map[string]BuildData

func (b *TestToBuildData) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	data, err := json.Marshal(b)
	if err != nil {
		res.Header().Set("Content-type", "text/html")
		res.WriteHeader(http.StatusInternalServerError)
		res.Write([]byte(fmt.Sprintf("<h3>Internal Error</h3><p>%v", err)))
		return
	}
	res.Header().Set("Content-type", "application/json")
	res.WriteHeader(http.StatusOK)
	res.Write(data)
}
