/*
Copyright 2022 The Kubernetes Authors.

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

// fakegerritserver serves github API for integration tests.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"

	gerrit "github.com/andygrunwald/go-gerrit"
	"k8s.io/test-infra/prow/interrupts"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/pjutil"
)

type options struct {
	port int
}

func (o *options) validate() error {
	return nil
}

func flagOptions() *options {
	o := &options{}
	flag.IntVar(&o.port, "port", 8888, "Port to listen on.")
	return o
}

func main() {
	logrusutil.ComponentInit()

	o := flagOptions()
	flag.Parse()
	if err := o.validate(); err != nil {
		logrus.WithError(err).Fatal("Invalid arguments.")
	}
	defer interrupts.WaitForGracefulShutdown()

	r := mux.NewRouter()

	r.Path("/").Handler(response(defaultHandler()))
	r.Path("/changes/{change-id}").Handler(response(changesHandler()))

	health := pjutil.NewHealth()
	health.ServeReady()

	logrus.Info("Start server")

	// setup done, actually start the server
	server := &http.Server{Addr: fmt.Sprintf(":%d", o.port), Handler: r}
	interrupts.ListenAndServe(server, 5*time.Second)
}

func response(f func(*http.Request) (interface{}, int, error)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		msg, statusCode, err := f(r)
		logrus.Infof("request: %s - %s. responses: %s, %d, %v", r.URL.Path, r.Method, msg, statusCode, err)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, err.Error())
			logrus.WithError(err).Errorf("failed serving %s ( %s )", r.URL.Path, r.Method)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Link", "")
		w.WriteHeader(statusCode)
		fmt.Fprint(w, msg)
		logrus.Info("Succeeded with request: ", statusCode)
	})
}

func defaultHandler() func(*http.Request) (interface{}, int, error) {
	return func(r *http.Request) (interface{}, int, error) {
		logrus.Infof("Not supported: %s, %s", r.URL.Path, r.Method)
		return "", http.StatusOK, nil
	}
}

func changesHandler() func(*http.Request) (interface{}, int, error) {
	return func(r *http.Request) (interface{}, int, error) {
		logrus.Infof("Serving: %s, %s", r.URL.Path, r.Method)
		vars := mux.Vars(r)
		id := vars["change-id"]
		if id == "1" {
			content, err := json.Marshal(gerrit.ChangeInfo{
				ChangeID: "1",
			})
			logrus.Debugf("JSON: %v", content)
			if err != nil {
				return "", http.StatusInternalServerError, err
			}
			return string(content), http.StatusOK, nil
		}
		return "", http.StatusNotFound, nil
	}
}
