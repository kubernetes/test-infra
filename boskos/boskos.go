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
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/boskos/ranch"
)

var configPath = flag.String("config", "resources.json", "Path to init resource file")
var storage = flag.String("storage", "", "Path to persistent volume to load the state")

func main() {
	flag.Parse()
	logrus.SetFormatter(&logrus.JSONFormatter{})

	r, err := ranch.NewRanch(*configPath, *storage)
	if err != nil {
		logrus.WithError(err).Fatalf("Failed to create ranch! Config: %v, storage : %v", *configPath, *storage)
	}

	http.Handle("/", handleDefault(r))
	http.Handle("/acquire", handleAcquire(r))
	http.Handle("/release", handleRelease(r))
	http.Handle("/reset", handleReset(r))
	http.Handle("/update", handleUpdate(r))
	http.Handle("/metric", handleMetric(r))

	go func() {
		logTick := time.NewTicker(time.Minute).C
		saveTick := time.NewTicker(time.Minute).C
		configTick := time.NewTicker(time.Minute * 10).C
		for {
			select {
			case <-logTick:
				r.LogStatus()
			case <-saveTick:
				r.SaveState()
			case <-configTick:
				r.SyncConfig(*configPath)
			}
		}
	}()

	logrus.Info("Start Service")
	logrus.WithError(http.ListenAndServe(":8080", nil)).Fatal("ListenAndServe returned.")
}

// ErrorToStatus translates error into http code
func ErrorToStatus(err error) int {
	switch err.(type) {
	default:
		return http.StatusInternalServerError
	case *ranch.OwnerNotMatch:
		return http.StatusUnauthorized
	case *ranch.ResourceNotFound:
		return http.StatusNotFound
	case *ranch.StateNotMatch:
		return http.StatusConflict
	}
}

//  handleDefault: Handler for /, always pass with 200
func handleDefault(r *ranch.Ranch) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		logrus.WithField("handler", "handleDefault").Infof("From %v", req.RemoteAddr)
	}
}

//  handleAcquire: Handler for /acquire
//  Method: POST
// 	URLParams:
//		Required: type=[string]  : type of requested resource
//		Required: state=[string] : current state of the requested resource
//		Required: dest=[string] : destination state of the requested resource
//		Required: owner=[string] : requester of the resource
func handleAcquire(r *ranch.Ranch) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		logrus.WithField("handler", "handleStart").Infof("From %v", req.RemoteAddr)

		if req.Method != "POST" {
			msg := fmt.Sprintf("Method %v, /acquire only accepts POST.", req.Method)
			logrus.Warning(msg)
			http.Error(res, msg, http.StatusMethodNotAllowed)
			return
		}

		// TODO(krzyzacy) - sanitize user input
		rtype := req.URL.Query().Get("type")
		state := req.URL.Query().Get("state")
		dest := req.URL.Query().Get("dest")
		owner := req.URL.Query().Get("owner")
		if rtype == "" || state == "" || dest == "" || owner == "" {
			msg := fmt.Sprintf("Type: %v, state: %v, dest: %v, owner: %v, all of them must be set in the request.", rtype, state, dest, owner)
			logrus.Warning(msg)
			http.Error(res, msg, http.StatusBadRequest)
			return
		}

		logrus.Infof("Request for a %v %v from %v, dest %v", state, rtype, owner, dest)

		resource, err := r.Acquire(rtype, state, dest, owner)

		if err != nil {
			logrus.WithError(err).Errorf("No available resource")
			http.Error(res, err.Error(), ErrorToStatus(err))
			return
		}

		resJSON, err := json.Marshal(resource)
		if err != nil {
			logrus.WithError(err).Errorf("json.Marshal failed: %v, resource will be released", resource)
			http.Error(res, err.Error(), ErrorToStatus(err))
			resource.Owner = "" // release the resource, though this is not expected to happen.
			return
		}
		logrus.Infof("Resource leased: %v", string(resJSON))
		fmt.Fprint(res, string(resJSON))
		return
	}
}

//  handleRelease: Handler for /release
//  Method: POST
//	URL Params:
//		Required: name=[string]  : name of finished resource
//		Required: owner=[string] : owner of the resource
//		Required: dest=[string]  : dest state
func handleRelease(r *ranch.Ranch) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		logrus.WithField("handler", "handleDone").Infof("From %v", req.RemoteAddr)

		if req.Method != "POST" {
			msg := fmt.Sprintf("Method %v, /release only accepts POST.", req.Method)
			logrus.Warning(msg)
			http.Error(res, msg, http.StatusMethodNotAllowed)
			return
		}

		name := req.URL.Query().Get("name")
		dest := req.URL.Query().Get("dest")
		owner := req.URL.Query().Get("owner")
		if name == "" || dest == "" || owner == "" {
			msg := fmt.Sprintf("Name: %v, dest: %v, owner: %v, all of them must be set in the request.", name, dest, owner)
			logrus.Warning(msg)
			http.Error(res, msg, http.StatusBadRequest)
			return
		}

		if err := r.Release(name, dest, owner); err != nil {
			logrus.WithError(err).Errorf("Done failed: %v - %v (from %v)", name, dest, owner)
			http.Error(res, err.Error(), ErrorToStatus(err))
			return
		}

		logrus.Infof("Done with resource %v, set to state %v", name, dest)
	}
}

//  handleReset: Handler for /reset
//  Method: POST
//	URL Params:
//		Required: type=[string] : type of resource in interest
//		Required: state=[string] : original state
//		Required: dest=[string] : dest state, for expired resource
//		Required: expire=[durationStr*] resource has not been updated since before {expire}.
func handleReset(r *ranch.Ranch) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		logrus.WithField("handler", "handleReset").Infof("From %v", req.RemoteAddr)

		if req.Method != "POST" {
			msg := fmt.Sprintf("Method %v, /reset only accepts POST.", req.Method)
			logrus.Warning(msg)
			http.Error(res, msg, http.StatusMethodNotAllowed)
			return
		}

		rtype := req.URL.Query().Get("type")
		state := req.URL.Query().Get("state")
		expireStr := req.URL.Query().Get("expire")
		dest := req.URL.Query().Get("dest")

		logrus.Infof("%v, %v, %v, %v", rtype, state, expireStr, dest)

		if rtype == "" || state == "" || expireStr == "" || dest == "" {
			msg := fmt.Sprintf("Type: %v, state: %v, expire: %v, dest: %v, all of them must be set in the request.", rtype, state, expireStr, dest)
			logrus.Warning(msg)
			http.Error(res, msg, http.StatusBadRequest)
			return
		}

		expire, err := time.ParseDuration(expireStr)
		if err != nil {
			logrus.WithError(err).Errorf("Invalid expiration: %v", expireStr)
			http.Error(res, err.Error(), http.StatusBadRequest)
			return
		}

		rmap := r.Reset(rtype, state, expire, dest)
		resJSON, err := json.Marshal(rmap)
		if err != nil {
			logrus.WithError(err).Errorf("json.Marshal failed: %v", rmap)
			http.Error(res, err.Error(), ErrorToStatus(err))
			return
		}
		logrus.Infof("Resource %v reset successful, %d items moved to state %v", rtype, len(rmap), dest)
		fmt.Fprint(res, string(resJSON))
	}
}

//  handleUpdate: Handler for /update
//  Method: POST
//  URLParams
//		Required: name=[string]  : name of target resource
//		Required: owner=[string] : owner of the resource
//		Required: state=[string] : current state of the resource
func handleUpdate(r *ranch.Ranch) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		logrus.WithField("handler", "handleUpdate").Infof("From %v", req.RemoteAddr)

		if req.Method != "POST" {
			msg := fmt.Sprintf("Method %v, /update only accepts POST.", req.Method)
			logrus.Warning(msg)
			http.Error(res, msg, http.StatusMethodNotAllowed)
			return
		}

		name := req.URL.Query().Get("name")
		owner := req.URL.Query().Get("owner")
		state := req.URL.Query().Get("state")
		if name == "" || owner == "" || state == "" {
			msg := fmt.Sprintf("Name: %v, owner: %v, state : %v, all of them must be set in the request.", name, owner, state)
			logrus.Warning(msg)
			http.Error(res, msg, http.StatusBadRequest)
			return
		}

		if err := r.Update(name, owner, state); err != nil {
			logrus.WithError(err).Errorf("Update failed: %v - %v (%v)", name, state, owner)
			http.Error(res, err.Error(), ErrorToStatus(err))
			return
		}

		logrus.Info("Updated resource %v", name)
	}
}

//  handleMetric: Handler for /metric
//  Method: GET
func handleMetric(r *ranch.Ranch) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		logrus.WithField("handler", "handleMetric").Infof("From %v", req.RemoteAddr)

		if req.Method != "GET" {
			logrus.Warning("[BadRequest]method %v, expect GET", req.Method)
			http.Error(res, "/metric only accepts GET", http.StatusMethodNotAllowed)
			return
		}

		rtype := req.URL.Query().Get("type")
		if rtype == "" {
			msg := "Type must be set in the request."
			logrus.Warning(msg)
			http.Error(res, msg, http.StatusBadRequest)
			return
		}

		metric, err := r.Metric(rtype)
		if err != nil {
			logrus.WithError(err).Errorf("Metric for %s failed", rtype)
			http.Error(res, err.Error(), ErrorToStatus(err))
			return
		}

		js, err := json.Marshal(metric)
		if err != nil {
			logrus.WithError(err).Error("Fail to marshal metric")
			http.Error(res, err.Error(), ErrorToStatus(err))
			return
		}

		res.Header().Set("Content-Type", "application/json")
		res.Write(js)
	}
}
