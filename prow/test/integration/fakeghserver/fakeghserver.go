/*
Copyright 2018 The Kubernetes Authors.

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

// fakeghserver serves github API for integration tests.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/google/go-github/github"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	prowgh "k8s.io/test-infra/prow/github"

	"k8s.io/test-infra/prow/github/fakegithub"
	"k8s.io/test-infra/prow/interrupts"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/pjutil"
)

var ()

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
	ghClient := fakegithub.NewFakeClient()

	r := mux.NewRouter()
	// So far, supports APIs used by crier:
	//type GitHubClient interface {
	// 	BotName() (string, error) # /user
	// 	CreateStatus(org, repo, ref string, s github.Status) error # fmt.Sprintf("/repos/%s/%s/statuses/%s", org, repo, SHA)
	// 	ListIssueComments(org, repo string, number int) ([]github.IssueComment, error) # fmt.Sprintf("/repos/%s/%s/issues/%d/comments", org, repo, number)
	// 	CreateComment(org, repo string, number int, comment string) error # fmt.Sprintf("/repos/%s/%s/issues/%d/comments", org, repo, number),
	// 	DeleteComment(org, repo string, ID int) error # fmt.Sprintf("/repos/%s/%s/issues/comments/%d", org, repo, number),
	// 	EditComment(org, repo string, ID int, comment string) error # fmt.Sprintf("/repos/%s/%s/issues/comments/%d", org, repo, number),
	r.Path("/").Handler(defaultHandler())
	r.Path("/user").Handler(userHandler(ghClient))
	r.Path("/repos/{org}/{repo}/statuses/{sha}").Handler(statusHandler(ghClient))
	r.Path("/repos/{org}/{repo}/commits/{sha}/status").Queries("per_page", "{page}").Handler(statusHandler(ghClient))
	r.Path("/repos/{org}/{repo}/issues").Handler(issueHandler(ghClient))
	r.Path("/repos/{org}/{repo}/issues/{issue_id}/comments").Handler(issueCommentHandler(ghClient))
	r.Path("/repos/{org}/{repo}/issues/comments/${comment_id}").Handler(issueCommentHandler(ghClient))

	health := pjutil.NewHealth()
	health.ServeReady()

	logrus.Info("Start server")

	// setup done, actually start the server
	server := &http.Server{Addr: ":8888", Handler: r}
	interrupts.ListenAndServe(server, 5*time.Second)
}

func unmarshal(w http.ResponseWriter, r *http.Request, data interface{}) error {
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()

	w.Header().Set("Content-Type", "application/json")
	err := d.Decode(&data)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "{\"error\": \"Failed unmarshal request: %v\"}", err.Error())
	}
	return err
}

func response(w http.ResponseWriter, r *http.Request, msg string, ok int, err error) error {
	logrus.Infof("request: %s - %s. responses: %s, %d, %v", r.URL.Path, r.Method, msg, ok, err)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, err.Error())
		logrus.Info(err)
		return err
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Link", "")
	w.WriteHeader(ok)
	fmt.Fprint(w, msg)
	logrus.Info("Succeeded with request: ", ok)
	return nil
}

func defaultHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logrus.Infof("Not supported: %s, %s", r.URL.Path, r.Method)
		if err := response(w, r, "", http.StatusNotFound,
			fmt.Errorf("{\"error\": \"API not supported\"}, %s, %s", r.URL.Path, r.Method)); err != nil {
			logrus.WithError(err).Error("failed serving default handler")
		}
	})
}

func userHandler(ghc *fakegithub.FakeClient) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logrus.Infof("Serving: %s, %s", r.URL.Path, r.Method)
		var msg string
		var ok int
		var err error
		ok = http.StatusOK
		userData, err := ghc.BotUser()
		if err == nil {
			var content []byte
			content, err = json.Marshal(&userData)
			msg = string(content)
		}
		if err := response(w, r, msg, ok, err); err != nil {
			logrus.WithError(err).Error("failed serving user handler")
		}
	})
}

func statusHandler(ghc *fakegithub.FakeClient) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logrus.Infof("Serving: %s, %s", r.URL.Path, r.Method)
		var msg string
		var ok int
		var err error
		ok = http.StatusOK
		vars := mux.Vars(r)
		org, repo, SHA := vars["org"], vars["repo"], vars["sha"]
		if r.Method == http.MethodPost {
			ok = http.StatusCreated
			data := prowgh.Status{}
			err = unmarshal(w, r, &data)
			if err == nil {
				err = ghc.CreateStatus(org, repo, SHA, data)
			}
		} else if r.Method == http.MethodGet {
			var res *prowgh.CombinedStatus
			// []prowgh.Status
			res, err = ghc.GetCombinedStatus(org, repo, SHA)
			if err == nil {
				var content []byte
				content, err = json.Marshal(res)
				if err == nil {
					msg = string(content)
				}
			}
		}
		if err := response(w, r, msg, ok, err); err != nil {
			logrus.WithError(err).Error("failed serving status handler")
		}
	})
}

func issueHandler(ghc *fakegithub.FakeClient) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logrus.Infof("Serving: %s, %s", r.URL.Path, r.Method)
		var msg string
		var ok int
		var err error
		vars := mux.Vars(r)
		org, repo := vars["org"], vars["repo"]
		// Create status is 201
		ok = http.StatusCreated
		data := github.Issue{}
		err = unmarshal(w, r, &data)
		if err == nil {
			var id int
			id, err = ghc.CreateIssue(org, repo, *data.Title, *data.Body, *data.Milestone.Number, nil, nil)
			msg = fmt.Sprintf("Issue %d created", id)
		}
		if err := response(w, r, msg, ok, err); err != nil {
			logrus.WithError(err).Error("failed serving issue handler")
		}
	})
}

func issueCommentHandler(ghc *fakegithub.FakeClient) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logrus.Infof("Serving: %s, %s", r.URL.Path, r.Method)
		var msg string
		var ok int
		var err error
		ok = http.StatusOK
		vars := mux.Vars(r)
		org, repo := vars["org"], vars["repo"]
		if issueID, exist := vars["issue_id"]; exist {
			var id int
			id, err = strconv.Atoi(issueID)
			if err == nil {
				if r.Method == http.MethodGet { // List
					var issues []prowgh.IssueComment
					issues, err = ghc.ListIssueComments(org, repo, id)
					var content []byte
					content, err = json.Marshal(issues)
					msg = string(content)
				} else if r.Method == http.MethodPost { // Create
					ok = http.StatusCreated
					data := prowgh.IssueComment{}
					err = unmarshal(w, r, &data)
					if err == nil {
						err = ghc.CreateComment(org, repo, id, data.Body)
					}
				} else {
					err = fmt.Errorf("{\"error\": \"API not supported\"}, %s, %s", r.URL.Path, r.Method)
				}
			}
		} else if commentID, exist := vars["comment_id"]; exist {
			var id int
			id, err = strconv.Atoi(commentID)
			if err == nil {
				if r.Method == http.MethodDelete { // Delete
					err = ghc.DeleteComment(org, repo, id)
				} else if r.Method == http.MethodPatch { // Edit
					content := &github.IssueComment{}
					err = unmarshal(w, r, content)
					if err == nil {
						err = ghc.EditComment(org, repo, id, *content.Body)
					}
				} else {
					err = fmt.Errorf("{\"error\": \"API not supported\"}, %s, %s", r.URL.Path, r.Method)
				}
			}
		}
		if err := response(w, r, msg, ok, err); err != nil {
			logrus.WithError(err).Error("failed serving user handler")
		}
	})
}
