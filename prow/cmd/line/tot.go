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
	"io/ioutil"
	"math/rand"
	"net/http"
	"strconv"
	"time"

	"github.com/Sirupsen/logrus"
)

var (
	retryDelay = 2 * time.Second
)

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
}

func getBuildID(server, name string) string {
	url := server + "/vend/" + name
	for retries := 0; retries < 30; retries++ {
		if retries > 0 {
			time.Sleep(retryDelay)
		}
		resp, err := http.Get(url)
		if err != nil {
			logrus.WithError(err).Warningf("unable to vend build id for %s (url %s)", name, url)
		}
		if resp.StatusCode != 200 {
			logrus.Warningf("unable to vend build id for %s (url %s): status code %d", name, url, resp.StatusCode)
			continue
		}
		buf, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			logrus.Warning(err)
			continue
		}
		return string(buf)
	}
	logrus.Errorf("unable to vend build id for %s (url %s)-- fallback", name, url)
	return strconv.Itoa(rand.Int())
}
