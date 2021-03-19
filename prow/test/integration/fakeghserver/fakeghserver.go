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

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	prowgh "k8s.io/test-infra/prow/github"

	"k8s.io/test-infra/prow/github/fakegithub"
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
	r.Path("/").Handler(response(defaultHandler()))
	r.Path("/user").Handler(response(userHandler(ghClient)))
	r.Path("/repos/{org}/{repo}/statuses/{sha}").Handler(response(statusHandler(ghClient)))
	r.Path("/repos/{org}/{repo}/commits/{sha}/status").Queries("per_page", "{page}").Handler(response(statusHandler(ghClient)))
	r.Path("/repos/{org}/{repo}/issues").Handler(response(issueHandler(ghClient)))
	r.Path("/repos/{org}/{repo}/issues/{issue_id}/comments").Handler(response(issueCommentHandler(ghClient)))
	r.Path("/repos/{org}/{repo}/issues/comments/${comment_id}").Handler(response(issueCommentHandler(ghClient)))

	health := pjutil.NewHealth()
	health.ServeReady()

	logrus.Info("Start server")

	// setup done, actually start the server
	server := &http.Server{Addr: ":8888", Handler: r}
	interrupts.ListenAndServe(server, 5*time.Second)
}

func unmarshal(r *http.Request, data interface{}) error {
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()

	if err := d.Decode(&data); err != nil {
		return fmt.Errorf("{\"error\": \"Failed unmarshal request: %v\"}", err.Error())
	}
	return nil
}

func response(f func(*http.Request) (string, int, error)) http.Handler {
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

func defaultHandler() func(*http.Request) (string, int, error) {
	return func(r *http.Request) (string, int, error) {
		logrus.Infof("Not supported: %s, %s", r.URL.Path, r.Method)
		return "", http.StatusNotFound,
			fmt.Errorf("{\"error\": \"API not supported\"}, %s, %s", r.URL.Path, r.Method)
	}
}

func userHandler(ghc *fakegithub.FakeClient) func(*http.Request) (string, int, error) {
	return func(r *http.Request) (string, int, error) {
		logrus.Infof("Serving: %s, %s", r.URL.Path, r.Method)
		userData, err := ghc.BotUser()
		if err != nil {
			return "", http.StatusInternalServerError, err
		}
		var content []byte
		content, err = json.Marshal(&userData)
		return string(content), http.StatusOK, err
	}
}

func statusHandler(ghc *fakegithub.FakeClient) func(*http.Request) (string, int, error) {
	return func(r *http.Request) (string, int, error) {
		logrus.Infof("Serving: %s, %s", r.URL.Path, r.Method)
		vars := mux.Vars(r)
		org, repo, SHA := vars["org"], vars["repo"], vars["sha"]
		if r.Method == http.MethodPost { // Create
			data := prowgh.Status{}
			if err := unmarshal(r, &data); err != nil {
				return "", http.StatusInternalServerError, err
			}
			return "", http.StatusCreated, ghc.CreateStatus(org, repo, SHA, data)
		}
		if r.Method == http.MethodGet {
			res, err := ghc.GetCombinedStatus(org, repo, SHA)
			if err != nil {
				return "", http.StatusInternalServerError, err
			}
			content, err := json.Marshal(res)
			if err != nil {
				return "", http.StatusInternalServerError, err
			}
			return string(content), http.StatusOK, nil
		}
		return "", http.StatusInternalServerError, fmt.Errorf("{\"error\": \"API not supported\"}, %s, %s", r.URL.Path, r.Method)
	}
}

func issueHandler(ghc *fakegithub.FakeClient) func(*http.Request) (string, int, error) {
	return func(r *http.Request) (string, int, error) {
		logrus.Infof("Serving: %s, %s", r.URL.Path, r.Method)
		vars := mux.Vars(r)
		org, repo := vars["org"], vars["repo"]
		data := prowgh.Issue{}
		if err := unmarshal(r, &data); err != nil {
			return "", http.StatusInternalServerError, err
		}
		id, err := ghc.CreateIssue(org, repo, data.Title, data.Body, data.Milestone.Number, nil, nil)
		return fmt.Sprintf("Issue %d created", id), http.StatusCreated, err
	}
}

func issueCommentHandler(ghc *fakegithub.FakeClient) func(*http.Request) (string, int, error) {
	return func(r *http.Request) (string, int, error) {
		logrus.Infof("Serving: %s, %s", r.URL.Path, r.Method)
		vars := mux.Vars(r)
		org, repo := vars["org"], vars["repo"]
		if issueID, exist := vars["issue_id"]; exist {
			id, err := strconv.Atoi(issueID)
			if err != nil {
				return "", http.StatusInternalServerError, err
			}
			if r.Method == http.MethodGet { // List
				var issues []prowgh.IssueComment
				issues, err = ghc.ListIssueComments(org, repo, id)
				if err != nil {
					return "", http.StatusInternalServerError, err
				}
				var content []byte
				content, err = json.Marshal(issues)
				return string(content), http.StatusOK, err
			}
			if r.Method == http.MethodPost { // Create
				data := prowgh.IssueComment{}
				if err = unmarshal(r, &data); err != nil {
					return "", http.StatusInternalServerError, err
				}
				return "", http.StatusCreated, ghc.CreateComment(org, repo, id, data.Body)
			}
		}
		if commentID, exist := vars["comment_id"]; exist {
			var id int
			id, err := strconv.Atoi(commentID)
			if err != nil {
				return "", http.StatusInternalServerError, err
			}
			if r.Method == http.MethodDelete { // Delete
				return "", http.StatusOK, ghc.DeleteComment(org, repo, id)
			}
			if r.Method == http.MethodPatch { // Edit
				content := &prowgh.IssueComment{}
				if err := unmarshal(r, content); err != nil {
					return "", http.StatusInternalServerError, err
				}
				return "", http.StatusOK, ghc.EditComment(org, repo, id, content.Body)
			}
		}
		return "", http.StatusInternalServerError, fmt.Errorf("{\"error\": \"API not supported\"}, %s, %s", r.URL.Path, r.Method)
	}
}
