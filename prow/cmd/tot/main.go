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

// Tot vends (rations) incrementing numbers for use in builds.
// https://en.wikipedia.org/wiki/Rum_ration
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/logrusutil"
)

type options struct {
	port        int
	storagePath string

	useFallback bool
	fallbackURI string
}

func gatherOptions() options {
	o := options{}
	flag.IntVar(&o.port, "port", 8888, "Port to listen on.")
	flag.StringVar(&o.storagePath, "storage", "tot.json", "Where to store the results.")

	flag.BoolVar(&o.useFallback, "fallback", false, "Fallback to GCS bucket for missing builds.")
	flag.StringVar(&o.fallbackURI, "fallback-url-template",
		"https://storage.googleapis.com/kubernetes-jenkins/logs/%s/latest-build.txt",
		"URL template to fallback to for every job that lacks a last vended build number.",
	)

	flag.Parse()
	return o
}

type store struct {
	Number       map[string]int // job name -> last vended build number
	mutex        sync.Mutex
	storagePath  string
	fallbackFunc func(string) int
}

func newStore(storagePath string) (*store, error) {
	s := &store{
		Number:      make(map[string]int),
		storagePath: storagePath,
	}
	buf, err := ioutil.ReadFile(storagePath)
	if err == nil {
		err = json.Unmarshal(buf, s)
		if err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	return s, nil
}

func (s *store) save() error {
	buf, err := json.Marshal(s)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(s.storagePath+".tmp", buf, 0644)
	if err != nil {
		return err
	}
	return os.Rename(s.storagePath+".tmp", s.storagePath)
}

func (s *store) vend(jobName string) int {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	n, ok := s.Number[jobName]
	if !ok && s.fallbackFunc != nil {
		n = s.fallbackFunc(jobName)
	}
	n++

	s.Number[jobName] = n

	err := s.save()
	if err != nil {
		logrus.Error(err)
	}

	return n
}

func (s *store) peek(jobName string) int {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.Number[jobName]
}

func (s *store) set(jobName string, n int) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.Number[jobName] = n

	err := s.save()
	if err != nil {
		logrus.Error(err)
	}
}

func (s *store) handle(w http.ResponseWriter, r *http.Request) {
	jobName := r.URL.Path[len("/vend/"):]
	switch r.Method {
	case "GET":
		n := s.vend(jobName)
		logrus.Infof("Vending %s number %d to %s.", jobName, n, r.RemoteAddr)
		fmt.Fprintf(w, "%d", n)
	case "HEAD":
		n := s.peek(jobName)
		logrus.Infof("Peeking %s number %d to %s.", jobName, n, r.RemoteAddr)
		fmt.Fprintf(w, "%d", n)
	case "POST":
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			logrus.WithError(err).Error("Unable to read body.")
			return
		}
		n, err := strconv.Atoi(string(body))
		if err != nil {
			logrus.WithError(err).Error("Unable to parse number.")
			return
		}
		logrus.Infof("Setting %s to %d from %s.", jobName, n, r.RemoteAddr)
		s.set(jobName, n)
	}
}

type fallbackHandler struct {
	template string
}

func (f fallbackHandler) get(jobName string) int {
	url := fmt.Sprintf(f.template, jobName)

	var body []byte

	for i := 0; i < 10; i++ {
		resp, err := http.Get(url)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				body, err = ioutil.ReadAll(resp.Body)
				if err == nil {
					break
				} else {
					logrus.WithError(err).Error("Failed to read response body.")
				}
			}
		} else {
			logrus.WithError(err).Errorf("Failed to GET %s.", url)
		}
		time.Sleep(2 * time.Second)
	}

	n, err := strconv.Atoi(strings.TrimSpace(string(body)))
	if err != nil {
		return 0
	}

	return n
}

func main() {
	o := gatherOptions()

	logrus.SetFormatter(
		logrusutil.NewDefaultFieldsFormatter(nil, logrus.Fields{"component": "tot"}),
	)

	s, err := newStore(o.storagePath)
	if err != nil {
		logrus.WithError(err).Fatal("newStore failed")
	}

	if o.useFallback {
		s.fallbackFunc = fallbackHandler{o.fallbackURI}.get
	}

	http.HandleFunc("/vend/", s.handle)

	logrus.Fatal(http.ListenAndServe(":"+strconv.Itoa(o.port), nil))
}
