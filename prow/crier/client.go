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

package crier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

func ReportToCrier(url string, r Report) error {
	var err error
	var resp *http.Response
	buf, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("error marshalling json: %v", err)
	}
	for retries := 0; retries < 20; retries++ {
		if retries > 0 {
			time.Sleep(5 * time.Second)
		}
		resp, err = http.Post(url+"/status", "application/json", bytes.NewBuffer(buf))
		if err != nil {
			continue
		}
		if resp.StatusCode != 200 {
			return fmt.Errorf("non-200 response from crier: %d", resp.StatusCode)
		} else {
			break
		}
	}
	return err
}
