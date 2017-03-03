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
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
)

var configPath = flag.String("config", "resources.json", "Path to init resource file")
var storage = flag.String("storage", "", "Path to presistent volume to load the state")

func main() {
	flag.Parse()
	logrus.SetFormatter(&logrus.JSONFormatter{})

	r, err := NewRanch(*configPath, *storage)
	if err != nil {
		logrus.WithError(err).Fatal("Fail to create my ranch!")
	}

	http.Handle("/", handleDefault())
	http.Handle("/start", handleStart(r))
	http.Handle("/done", handleDone(r))
	http.Handle("/reset", handleReset(r))
	http.Handle("/update", handleUpdate(r))
	http.Handle("/metric", handleMetric(r))

	logrus.Info("Start Service")
	logrus.WithError(http.ListenAndServe(":8080", nil)).Fatal("ListenAndServe returned.")
}

func handleDefault() http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		logrus.WithField("handler", "handleDefault").Infof("From %v", req.RemoteAddr)
	}
}

func handleStart(r *Ranch) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		logrus.WithField("handler", "handleStart").Infof("From %v", req.RemoteAddr)

		rtype := req.URL.Query().Get("type")
		state := req.URL.Query().Get("state")
		owner := req.URL.Query().Get("owner")
		if rtype == "" || state == "" || owner == "" {
			logrus.Warning("[BadRequest]type, state and owner must present in the request")
			http.Error(res, "Type, state and owner must present in the request", http.StatusBadRequest)
			return
		}

		logrus.Infof("Request for a %v %v from %v", state, rtype, owner)

		resource := r.Request(rtype, state, owner)

		if resource != nil {
			resJSON, err := json.Marshal(resource)
			if err != nil {
				logrus.WithError(err).Errorf("json.Marshal failed : %v", resource)
				http.Error(res, err.Error(), http.StatusInternalServerError)
				return
			}
			logrus.Infof("Resource leased: %v", string(resJSON))
			fmt.Fprint(res, string(resJSON))
		} else {
			logrus.Infof("No available resource")
			http.Error(res, "No resource", http.StatusInternalServerError)
		}
	}
}

func handleDone(r *Ranch) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		logrus.WithField("handler", "handleDone").Infof("From %v", req.RemoteAddr)

		name := req.URL.Query().Get("name")
		state := req.URL.Query().Get("state")
		if name == "" || state == "" {
			logrus.Warning("[BadRequest]name and state must present in the request")
			http.Error(res, "Name and state must present in the request", http.StatusBadRequest)
			return
		}

		if err := r.Done(name, state); err != nil {
			logrus.WithError(err).Errorf("Done failed: %v - %v", name, state)
			http.Error(res, err.Error(), http.StatusInternalServerError)
			return
		}

		logrus.Infof("Done with resource %v, set to state %v", name, state)
	}
}

func handleReset(r *Ranch) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		logrus.WithField("handler", "handleReset").Infof("From %v", req.RemoteAddr)

		rtype := req.URL.Query().Get("type")
		state := req.URL.Query().Get("state")
		expireStr := req.URL.Query().Get("expire")
		dest := req.URL.Query().Get("dest")

		logrus.Infof("%v, %v, %v, %v", rtype, state, expireStr, dest)

		if rtype == "" || state == "" || expireStr == "" || dest == "" {
			logrus.Warning("[BadRequest]type, state, expire and dest must present in the request")
			http.Error(res, "Type, state, expire and dest must present in the request", http.StatusBadRequest)
			return
		}

		expire, err := time.ParseDuration(expireStr)
		if err != nil {
			logrus.WithError(err).Errorf("Invalid expire: %v", expireStr)
			http.Error(res, err.Error(), http.StatusBadRequest)
			return
		}

		rmap := r.Reset(rtype, state, expire, dest)
		resJSON, err := json.Marshal(rmap)
		if err != nil {
			logrus.WithError(err).Errorf("json.Marshal failed : %v", rmap)
			http.Error(res, err.Error(), http.StatusInternalServerError)
			return
		}
		logrus.Infof("Resource reset: %v", string(resJSON))
		fmt.Fprint(res, string(resJSON))
	}
}

func handleUpdate(r *Ranch) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		logrus.WithField("handler", "handleUpdate").Infof("From %v", req.RemoteAddr)

		// validate vars
		name := req.URL.Query().Get("name")
		if name == "" {
			logrus.Warning("[BadRequest]name must present in the request")
			http.Error(res, "Name must present in the request", http.StatusBadRequest)
			return
		}

		if err := r.Update(name); err != nil {
			logrus.WithError(err).Errorf("Update failed: %v", name)
			http.Error(res, err.Error(), http.StatusInternalServerError)
			return
		}

		logrus.Info("Updated resource %v", name)
	}
}

func handleMetric(r *Ranch) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		fmt.Fprint(res, "To be implemented.\n")
	}
}

type Resource struct {
	Type       string    `json:"type"`
	Name       string    `json:"name"`
	State      string    `json:"state"`
	Owner      string    `json:"owner"`
	LastUpdate time.Time `json:"lastupdate"`
}

type Ranch struct {
	Resources   []*Resource
	lock        sync.RWMutex
	storagePath string
}

func NewRanch(config string, storage string) (*Ranch, error) {

	newRanch := &Ranch{
		storagePath: storage,
	}

	if storage != "" {
		buf, err := ioutil.ReadFile(storage)
		if err == nil {
			logrus.Infof("Current state: %v.", buf)
			err = json.Unmarshal(buf, newRanch)
			if err != nil {
				return nil, err
			}
		} else if !os.IsNotExist(err) {
			return nil, err
		}
	}

	file, err := ioutil.ReadFile(config)
	if err != nil {
		return nil, err
	}

	var data []Resource
	err = json.Unmarshal(file, &data)
	if err != nil {
		return nil, err
	}

	for _, p := range data {
		found := false
		for _, exist := range newRanch.Resources {
			if p.Name == exist.Name {
				found = true
				break
			}
		}
		if !found {
			if p.State == "" {
				p.State = "free"
			}
			p.LastUpdate = time.Now()
			newRanch.Resources = append(newRanch.Resources, &p)
		}
	}
	newRanch.logStatus()

	return newRanch, nil
}

func (r *Ranch) logStatus() {
	r.lock.RLock()
	defer r.lock.RUnlock()

	for _, res := range r.Resources {
		resJSON, _ := json.Marshal(res)
		logrus.Infof("Current Resources : %v", string(resJSON))
	}
}

func (r *Ranch) saveState() {
	if r.storagePath == "" {
		return
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	// If fail to save data, fatal and restart the server
	buf, err := json.Marshal(r)
	if err != nil {
		logrus.WithError(err).Fatal("Error marshal ranch")
	}
	err = ioutil.WriteFile(r.storagePath+".tmp", buf, 0644)
	if err != nil {
		logrus.WithError(err).Fatal("Error write file")
	}
	err = os.Rename(r.storagePath+".tmp", r.storagePath)
	if err != nil {
		logrus.WithError(err).Fatal("Error rename file")
	}
}

func (r *Ranch) Request(rtype string, state string, owner string) *Resource {
	r.lock.Lock()
	defer r.lock.Unlock()

	for _, res := range r.Resources {
		if rtype == res.Type && state == res.State && res.Owner == "" {
			res.LastUpdate = time.Now()
			res.Owner = owner
			return res
		}
	}

	return nil
}

func (r *Ranch) Done(name string, state string) error {
	r.lock.Lock()
	defer r.lock.Unlock()

	for _, res := range r.Resources {
		if name == res.Name {
			if res.Owner == "" {
				return fmt.Errorf("Resource %v should have an owner!", res.Name)
			}
			res.LastUpdate = time.Now()
			res.Owner = ""
			res.State = state
			return nil
		}
	}

	return fmt.Errorf("Cannot find resource %v", name)
}

func (r *Ranch) Update(name string) error {
	r.lock.Lock()
	defer r.lock.Unlock()

	for _, res := range r.Resources {
		if name == res.Name {
			res.LastUpdate = time.Now()
			return nil
		}
	}

	return fmt.Errorf("Cannot find resource %v", name)
}

func (r *Ranch) Reset(rtype string, state string, expire time.Duration, dest string) map[string]string {
	r.lock.Lock()
	defer r.lock.Unlock()

	ret := make(map[string]string)

	for _, res := range r.Resources {
		if rtype == res.Type && state == res.State && res.Owner != "" {
			if time.Now().Sub(res.LastUpdate) > expire {
				res.LastUpdate = time.Now()
				ret[res.Name] = res.Owner
				res.Owner = ""
				res.State = dest
			}
		}
	}

	return ret
}
