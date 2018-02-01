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
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"k8s.io/test-infra/prow/userdashboard"
)

const dataCacheLife = time.Minute

type userAgent struct {
	path string

	sync.Mutex
	data   *userdashboard.UserData
	expiry time.Time
}

func (ua *userAgent) getData() (*userdashboard.UserData, error) {
	ua.Lock()
	defer ua.Unlock()
	if time.Now().Before(ua.expiry) {
		return ua.data, nil
	}
	var data userdashboard.UserData
	resp, err := http.Get(ua.path)
	if err != nil {
		return nil, fmt.Errorf("error GETing user dashboard data: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("response has status code %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("error decoding json user dashboard data: %v", err)
	}

	ua.data = &data
	ua.expiry = time.Now().Add(dataCacheLife)
	return ua.data, nil
}
