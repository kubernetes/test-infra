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
	"sync"

	log "github.com/Sirupsen/logrus"
)

var (
	port        = flag.Int("port", 8888, "port to listen on")
	logJson     = flag.Bool("log-json", false, "output log in JSON format")
	storagePath = flag.String("storage", "tot.json", "where to store the results")
)

type store struct {
	Number      map[string]int
	mutex       sync.Mutex
	storagePath string
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
	n := s.Number[b] + 1
	s.Number[b] = n

	err := s.save()
	if err != nil {
		log.Error(err)
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
		log.Fatal(err)
	}

	http.HandleFunc("/vend/", func(w http.ResponseWriter, r *http.Request) {
		b := r.URL.Path[len("/vend/"):]
		n := s.vend(b)
		log.Infof("sending %s number %d to %s", b, n, r.RemoteAddr)
		fmt.Fprintf(w, "%d", n)
	})

	log.Fatal(http.ListenAndServe(":"+strconv.Itoa(*port), nil))
}
