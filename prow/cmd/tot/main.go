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

	log "github.com/Sirupsen/logrus"
)

var (
	port        = flag.Int("port", 8888, "port to listen on")
	logJson     = flag.Bool("log-json", false, "output log in JSON format")
	storagePath = flag.String("storage", "tot.json", "where to store the results")

	// TODO(rmmh): remove this once we have no jobs running on Jenkins
	useFallback = flag.Bool("fallback", false, "fallback to GCS bucket for missing builds")
	fallbackURI = "https://storage.googleapis.com/kubernetes-jenkins/logs/%s/latest-build.txt"
)

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
	err = os.Rename(s.storagePath+".tmp", s.storagePath)
	if err != nil {
		return err
	}
	return nil
}

func (s *store) vend(b string) int {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	n, ok := s.Number[b]
	if !ok && s.fallbackFunc != nil {
		n = s.fallbackFunc(b)
	}
	n++

	s.Number[b] = n

	err := s.save()
	if err != nil {
		log.Error(err)
	}

	return n
}

func (s *store) peek(b string) int {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.Number[b]
}

func (s *store) set(b string, n int) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.Number[b] = n

	err := s.save()
	if err != nil {
		log.Error(err)
	}
}

func (s *store) handle(w http.ResponseWriter, r *http.Request) {
	b := r.URL.Path[len("/vend/"):]
	switch r.Method {
	case "GET":
		n := s.vend(b)
		log.Infof("vending %s number %d to %s", b, n, r.RemoteAddr)
		fmt.Fprintf(w, "%d", n)
	case "HEAD":
		n := s.peek(b)
		log.Infof("peeking %s number %d to %s", b, n, r.RemoteAddr)
		fmt.Fprintf(w, "%d", n)
	case "POST":
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			log.WithError(err).Error("unable to read body")
			return
		}
		n, err := strconv.Atoi(string(body))
		if err != nil {
			log.WithError(err).Error("unable to parse number")
			return
		}
		log.Infof("setting %s to %d from %s", b, n, r.RemoteAddr)
		s.set(b, n)
	}
}

type fallbackHandler struct {
	template string
}

func (f fallbackHandler) get(b string) int {
	url := fmt.Sprintf(f.template, b)

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
					log.WithError(err).Error("Failed to read response body.")
				}
			}
		} else {
			log.WithError(err).Errorf("Failed to GET %s", url)
		}
		time.Sleep(2)
	}

	n, err := strconv.Atoi(strings.TrimSpace(string(body)))
	if err != nil {
		return 0
	}

	return n
}

func main() {
	flag.Parse()

	if *logJson {
		log.SetFormatter(&log.JSONFormatter{})
	}
	log.SetLevel(log.DebugLevel)

	s, err := newStore(*storagePath)
	if err != nil {
		log.WithError(err).Fatal("newStore failed")
	}

	if *useFallback {
		s.fallbackFunc = fallbackHandler{fallbackURI}.get
	}

	http.HandleFunc("/vend/", s.handle)

	log.Fatal(http.ListenAndServe(":"+strconv.Itoa(*port), nil))
}
