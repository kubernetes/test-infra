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

package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/boskos/common"
	"k8s.io/test-infra/boskos/ranch"
	"k8s.io/test-infra/prow/simplifypath"
)

// l keeps the tree legible
func l(fragment string, children ...simplifypath.Node) simplifypath.Node {
	return simplifypath.L(fragment, children...)
}

// NewBoskosSimplifier returns a Simplifier for all Boskos URIs to be used with metrics collection.
func NewBoskosSimplifier() simplifypath.Simplifier {
	return simplifypath.NewSimplifier(l("", // shadow element mimicing the root
		l("acquire"),
		l("acquirebystate"),
		l("release"),
		l("reset"),
		l("update"),
		l("metric"),
	))
}

//NewBoskosHandler constructs the boskos handler.
func NewBoskosHandler(r *ranch.Ranch) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/", handleDefault(r))
	mux.Handle("/acquire", handleAcquire(r))
	mux.Handle("/acquirebystate", handleAcquireByState(r))
	mux.Handle("/release", handleRelease(r))
	mux.Handle("/reset", handleReset(r))
	mux.Handle("/update", handleUpdate(r))
	mux.Handle("/metric", handleMetric(r))
	return mux
}

type badRequestError string

func (bre badRequestError) Error() string { return string(bre) }

// errorToStatus translates error into http code
func errorToStatus(err error) int {
	switch err.(type) {
	default:
		return http.StatusInternalServerError
	case *ranch.OwnerNotMatch:
		return http.StatusUnauthorized
	case *ranch.ResourceNotFound:
		return http.StatusNotFound
	case *ranch.ResourceTypeNotFound:
		return http.StatusNotFound
	case *ranch.StateNotMatch:
		return http.StatusConflict
	case badRequestError:
		return http.StatusBadRequest
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

		if req.Method != http.MethodPost {
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
		requestID := req.URL.Query().Get("request_id")
		if rtype == "" || state == "" || dest == "" || owner == "" {
			bre := badRequestError(fmt.Sprintf("Type: %v, state: %v, dest: %v, owner: %v, all of them must be set in the request.", rtype, state, dest, owner))
			returnAndLogError(res, bre, "Bad request")
			return
		}

		logrus.Infof("Request for a %v %v from %v, dest %v", state, rtype, owner, dest)

		resource, err := r.Acquire(rtype, state, dest, owner, requestID)
		if err != nil {
			returnAndLogError(res, err, "Acquire failed")
			return
		}

		resJSON, err := json.Marshal(resource.ToResource())
		if err != nil {
			logrus.WithError(err).Errorf("json.Marshal failed: %v, resource will be released", resource)
			http.Error(res, err.Error(), errorToStatus(err))
			// release the resource, though this is not expected to happen.
			err = r.Release(resource.Name, state, owner)
			if err != nil {
				logrus.WithError(err).Warningf("unable to release resource %s", resource.Name)
			}
			return
		}
		logrus.Infof("Resource leased: %v", string(resJSON))
		fmt.Fprint(res, string(resJSON))
	}
}

//  handleAcquireByState: Handler for /acquirebystate
//  Method: POST
// 	URLParams:
//		Required: state=[string] : current state of the requested resource
//		Required: dest=[string]  : destination state of the requested resource
//		Required: owner=[string] : requester of the resource
//		Required: names=[string] : expected resources names
func handleAcquireByState(r *ranch.Ranch) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		logrus.WithField("handler", "handleStart").Infof("From %v", req.RemoteAddr)

		if req.Method != http.MethodPost {
			msg := fmt.Sprintf("Method %v, /acquire only accepts POST.", req.Method)
			logrus.Warning(msg)
			http.Error(res, msg, http.StatusMethodNotAllowed)
			return
		}

		// TODO(krzyzacy) - sanitize user input
		state := req.URL.Query().Get("state")
		dest := req.URL.Query().Get("dest")
		owner := req.URL.Query().Get("owner")
		names := req.URL.Query().Get("names")
		if state == "" || dest == "" || owner == "" || names == "" {
			msg := fmt.Sprintf(
				"state: %v, dest: %v, owner: %v, names: %v - all of them must be set in the request.",
				state, dest, owner, names)
			logrus.Warning(msg)
			http.Error(res, msg, http.StatusBadRequest)
			return
		}
		rNames := strings.Split(names, ",")
		logrus.Infof("Request resources %s at state %v from %v, to state %v",
			strings.Join(rNames, ", "), state, owner, dest)

		resources, err := r.AcquireByState(state, dest, owner, rNames)

		if err != nil {
			returnAndLogError(res, err, "AcquireByState")
			return
		}

		var apiResources []common.Resource
		for _, resource := range resources {
			apiResources = append(apiResources, resource.ToResource())
		}

		resBytes := new(bytes.Buffer)

		if err := json.NewEncoder(resBytes).Encode(apiResources); err != nil {
			logrus.WithError(err).Errorf("json.Marshal failed: %v, resources will be released", apiResources)
			http.Error(res, err.Error(), errorToStatus(err))
			for _, resource := range resources {
				err := r.Release(resource.Name, state, owner)
				if err != nil {
					logrus.WithError(err).Warningf("unable to release resource %s", resource.Name)
				}
			}
			return
		}
		logrus.Infof("Resource leased: %v", resBytes.String())
		fmt.Fprint(res, resBytes.String())
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

		if req.Method != http.MethodPost {
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
			returnAndLogError(res, err, fmt.Sprintf("Done failed: %v - %v (from %v)", name, dest, owner))
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

		if req.Method != http.MethodPost {
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
			logrus.WithError(err).Debugf("Invalid expiration: %v", expireStr)
			http.Error(res, err.Error(), http.StatusBadRequest)
			return
		}

		rmap, err := r.Reset(rtype, state, expire, dest)
		if err != nil {
			returnAndLogError(res, err, "could not reset states")
			return
		}
		resJSON, err := json.Marshal(rmap)
		if err != nil {
			logrus.WithError(err).Errorf("json.Marshal failed: %v", rmap)
			http.Error(res, err.Error(), errorToStatus(err))
			return
		}
		logrus.Infof("Resource %v reset successful, %d items moved to state %v", rtype, len(rmap), dest)
		fmt.Fprint(res, string(resJSON))
	}
}

//  handleUpdate: Handler for /update
//  Method: POST
//  URLParams
//		Required: name=[string]              : name of target resource
//		Required: owner=[string]             : owner of the resource
//		Required: state=[string]             : current state of the resource
//		Optional: userData=[common.UserData] : user data id to update
func handleUpdate(r *ranch.Ranch) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		logrus.WithField("handler", "handleUpdate").Infof("From %v", req.RemoteAddr)

		if req.Method != http.MethodPost {
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

		var userData common.UserData

		if req.Body != nil {
			err := json.NewDecoder(req.Body).Decode(&userData)
			switch {
			case err == io.EOF:
				// empty body
			case err != nil:
				logrus.WithError(err).Warning("Unable to read from response body")
				http.Error(res, err.Error(), http.StatusBadRequest)
				return
			}
		}

		if err := r.Update(name, owner, state, &userData); err != nil {
			returnAndLogError(res, err, fmt.Sprintf("Update failed: %v - %v (%v)", name, state, owner))
			return
		}

		logrus.Infof("Updated resource %v", name)
	}
}

//  handleMetric: Handler for /metric
//  Method: GET
func handleMetric(r *ranch.Ranch) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		logrus.WithField("handler", "handleMetric").Infof("From %v", req.RemoteAddr)

		if req.Method != http.MethodGet {
			logrus.Warningf("[BadRequest]method %v, expect GET", req.Method)
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
			http.Error(res, err.Error(), errorToStatus(err))
			return
		}

		js, err := json.Marshal(metric)
		if err != nil {
			logrus.WithError(err).Error("Fail to marshal metric")
			http.Error(res, err.Error(), errorToStatus(err))
			return
		}

		res.Header().Set("Content-Type", "application/json")
		res.Write(js)
	}
}

func returnAndLogError(res http.ResponseWriter, err error, logMsg string) {
	logMsg = fmt.Sprintf("%s: %v", logMsg, err)
	log := logrus.WithError(err)
	httpStatus := errorToStatus(err)
	if httpStatus > 499 {
		log.Error(logMsg)
	} else {
		log.Debug(logMsg)
	}
	http.Error(res, logMsg, httpStatus)
}
