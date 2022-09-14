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
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"

	gerrit "github.com/andygrunwald/go-gerrit"
	"k8s.io/test-infra/prow/gerrit/fakegerrit"
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

	rMain := mux.NewRouter()
	// When authenticated the request URL has a prefix of `/a`, also handle this case.
	rAuthed := rMain.PathPrefix("/a").Subrouter()
	fakeClient := fakegerrit.NewFakeGerritClient()

	rMain.Path("/").Handler(response(defaultHandler()))

	// Handle authenticated and non-authenticated requests the same way for now.
	for _, r := range []*mux.Router{rMain, rAuthed} {
		//GetChange GET
		r.Path("/changes/{change-id}").Handler(response(changesHandler(fakeClient)))
		// SetReview POST
		r.Path("/changes/{change-id}/revisions/{revision-id}/review").Handler(response(changesHandler(fakeClient)))
		// QueryChanges GET
		r.Path("/changes/").Handler(response(handleQueryChanges(fakeClient)))
		// ListChangeComments GET
		r.Path("/changes/{change-id}/comments").Handler(response(handleGetComments(fakeClient)))

		// GetAccount GET
		r.Path("/accounts/{account-id}").Handler(response(accountHandler(fakeClient)))
		// SetUsername PUT
		r.Path("/accounts/{account-id}/username").Handler(response(accountHandler(fakeClient)))

		// GetBranch GET
		r.Path("/projects/{project-name}/branches/{branch-id}").Handler(response(projectHandler(fakeClient)))

		// Use to populate the server for testing
		r.Path("/admin/add/change/{project}").Handler(response(addChangeHandler(fakeClient)))
		r.Path("/admin/add/branch/{project}/{branch-name}").Handler(response(addBranchHandler(fakeClient)))
		r.Path("/admin/add/account").Handler(response(addAccountHandler(fakeClient)))
		r.Path("/admin/login/{id}").Handler(response(loginHandler(fakeClient)))
		r.Path("/admin/reset").Handler(response(resetHandler(fakeClient)))
	}

	health := pjutil.NewHealth()
	health.ServeReady()

	logrus.Info("Start server")

	// setup done, actually start the server
	server := &http.Server{Addr: fmt.Sprintf(":%d", o.port), Handler: rMain}
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

// GetBranch
func projectHandler(fgc *fakegerrit.FakeGerrit) func(*http.Request) (interface{}, int, error) {
	return func(r *http.Request) (interface{}, int, error) {
		vars := mux.Vars(r)
		projectName := vars["project-name"]
		branchID := vars["branch-id"]
		if res := fgc.GetBranch(projectName, branchID); res != nil {
			content, err := json.Marshal(res)
			if err != nil {
				return "", http.StatusInternalServerError, err
			}
			return string(content), http.StatusOK, nil
		}
		return "branch does not exist", http.StatusNotFound, nil
	}
}

// Admin endpoint to add a change to the Fake Gerrit Server
func addChangeHandler(fgc *fakegerrit.FakeGerrit) func(*http.Request) (interface{}, int, error) {
	return func(r *http.Request) (interface{}, int, error) {
		vars := mux.Vars(r)
		project := vars["project"]
		change := gerrit.ChangeInfo{}
		if err := unmarshal(r, &change); err != nil {
			logrus.Infof("Error unmarshaling: %v", err)
			return "", http.StatusInternalServerError, err
		}
		fgc.AddChange(project, &change)
		return "", http.StatusOK, nil
	}
}

// Admin endpoint to add a change to the Fake Gerrit Server
func addAccountHandler(fgc *fakegerrit.FakeGerrit) func(*http.Request) (interface{}, int, error) {
	return func(r *http.Request) (interface{}, int, error) {
		account := gerrit.AccountInfo{}
		if err := unmarshal(r, &account); err != nil {
			logrus.Infof("Error unmarshaling: %v", err)
			return "", http.StatusInternalServerError, err
		}
		fgc.AddAccount(&account)
		return "", http.StatusOK, nil
	}
}

// Admin endpoint to add a change to the Fake Gerrit Server
func loginHandler(fgc *fakegerrit.FakeGerrit) func(*http.Request) (interface{}, int, error) {
	return func(r *http.Request) (interface{}, int, error) {
		vars := mux.Vars(r)
		id := vars["id"]

		if err := fgc.SetSelf(id); err != nil {
			return "", http.StatusForbidden, fmt.Errorf("unable to login. ID %s does not exist", id)
		}
		return "", http.StatusOK, nil
	}
}

// Admin endpoint to add a change to the Fake Gerrit Server
func addBranchHandler(fgc *fakegerrit.FakeGerrit) func(*http.Request) (interface{}, int, error) {
	return func(r *http.Request) (interface{}, int, error) {
		vars := mux.Vars(r)
		branchName := vars["branch-name"]
		project := vars["project"]
		branch := gerrit.BranchInfo{}
		if err := unmarshal(r, &branch); err != nil {
			logrus.Infof("Error unmarshaling: %v", err)
			return "", http.StatusInternalServerError, err
		}
		fgc.AddBranch(project, branchName, &branch)
		return "", http.StatusOK, nil
	}
}

// Admin endpoint to reset the Fake Gerrit Server
func resetHandler(fgc *fakegerrit.FakeGerrit) func(*http.Request) (interface{}, int, error) {
	return func(r *http.Request) (interface{}, int, error) {
		fgc.Reset()
		return "", http.StatusOK, nil
	}
}

// Handles ListChangeComments
func handleGetComments(fgc *fakegerrit.FakeGerrit) func(*http.Request) (interface{}, int, error) {
	return func(r *http.Request) (interface{}, int, error) {
		logrus.Infof("Serving: %s, %s", r.URL.Path, r.Method)
		vars := mux.Vars(r)
		id := vars["change-id"]
		comments := fgc.GetComments(id)
		if comments == nil {
			return "change-id must be provided", http.StatusNotFound, nil
		}
		content, err := json.Marshal(comments)
		if err != nil {
			return "", http.StatusInternalServerError, err
		}
		return string(content), http.StatusOK, nil
	}
}

func processQueryString(query string) string {
	return strings.TrimPrefix(query, "project:")
}

// Handles QueryChanges
func handleQueryChanges(fgc *fakegerrit.FakeGerrit) func(*http.Request) (interface{}, int, error) {
	return func(r *http.Request) (interface{}, int, error) {
		logrus.Infof("Serving: %s, %s", r.URL.Path, r.Method)
		query := r.URL.Query().Get("q")
		start := r.URL.Query().Get("start")
		if start == "" {
			start = "0"
		}
		startint, err := strconv.Atoi(start)
		if err != nil {
			return "", http.StatusInternalServerError, err
		}
		project := processQueryString(query)

		logrus.Infof("Query: %s, Project: %s", query, project)
		if project == "" {
			return "project must be provided as query string: 'q=project:<PROJECT>'", http.StatusNotFound, nil
		}

		res := fgc.GetChangesForProject(project, startint, 100)
		content, err := json.Marshal(res)
		if err != nil {
			return "", http.StatusInternalServerError, err
		}
		return string(content), http.StatusOK, nil
	}
}

// Handles GetAccount and SetUsername
func accountHandler(fgc *fakegerrit.FakeGerrit) func(*http.Request) (interface{}, int, error) {
	return func(r *http.Request) (interface{}, int, error) {
		logrus.Infof("Serving: %s, %s", r.URL.Path, r.Method)
		vars := mux.Vars(r)
		id := vars["account-id"]
		account := fgc.GetAccount(id)
		if account == nil {
			return "account cannot be empty", http.StatusNotFound, nil
		}
		// SetUsername
		if r.Method == http.MethodPut {
			if account.Username != "" {
				return "", http.StatusMethodNotAllowed, nil
			}
			username := gerrit.UsernameInput{}
			if err := unmarshal(r, &username); err != nil {
				return "", http.StatusInternalServerError, err
			}

			fgc.Accounts[id].Username = username.Username
			return username.Username, http.StatusOK, nil
		}
		// GetAccount
		content, err := json.Marshal(account)
		if err != nil {
			return "", http.StatusInternalServerError, err
		}
		logrus.Debugf("JSON: %v", content)
		return string(content), http.StatusOK, nil
	}
}

// Handles GetChange and SetReview
func changesHandler(fgc *fakegerrit.FakeGerrit) func(*http.Request) (interface{}, int, error) {
	return func(r *http.Request) (interface{}, int, error) {
		logrus.Infof("Serving: %s, %s", r.URL.Path, r.Method)
		vars := mux.Vars(r)
		id := vars["change-id"]
		change := fgc.GetChange(id)
		if change == nil {
			return "", http.StatusMisdirectedRequest, nil
		}
		if r.Method == http.MethodPost {
			review := gerrit.ReviewInput{}
			if err := unmarshal(r, &review); err != nil {
				return "", http.StatusInternalServerError, err
			}
			change.Messages = append(change.Messages, gerrit.ChangeMessageInfo{Message: review.Message})
			// GetChange
		} else {
			content, err := json.Marshal(change)
			if err != nil {
				return "", http.StatusInternalServerError, err
			}
			logrus.Debugf("JSON: %v", content)
			return string(content), http.StatusOK, nil
		}
		return "", http.StatusForbidden, nil
	}
}

func unmarshal(r *http.Request, data interface{}) error {
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()

	if err := d.Decode(&data); err != nil {
		return fmt.Errorf("{\"error\": \"Failed unmarshal request: %v\"}", err.Error())
	}

	logrus.Infof("Output of Unmarshal: %v", data)
	return nil
}
