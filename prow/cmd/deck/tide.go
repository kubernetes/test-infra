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
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/tide"
)

type tideData struct {
	Queries     []string
	TideQueries []config.TideQuery
	Pools       []tide.Pool
}

type tideAgent struct {
	log  *logrus.Entry
	path string

	sync.Mutex
	pools []tide.Pool
}

func (ta *tideAgent) start() {
	if err := ta.update(); err != nil {
		ta.log.WithError(err).Error("Updating pool for the first time.")
	}
	go func() {
		for range time.Tick(time.Second * 10) {
			if err := ta.update(); err != nil {
				ta.log.WithError(err).Error("Updating pool.")
			}
		}
	}()
}

func (ta *tideAgent) update() error {
	var pools []tide.Pool
	var resp *http.Response
	var err error
	for i := 0; i < 3; i++ {
		if err != nil {
			ta.log.WithError(err).Warning("Tide request failed. Retrying.")
			time.Sleep(5 * time.Second)
		}
		resp, err = http.Get(ta.path)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode < 200 || resp.StatusCode > 299 {
				err = fmt.Errorf("response has status code %d", resp.StatusCode)
				continue
			}
			if err := json.NewDecoder(resp.Body).Decode(&pools); err != nil {
				return err
			}
			break
		}
	}
	if err != nil {
		return err
	}
	ta.Lock()
	defer ta.Unlock()
	ta.pools = pools
	return nil
}
