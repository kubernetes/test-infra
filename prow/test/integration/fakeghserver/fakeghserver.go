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
	"strings"
	"time"

	"github.com/google/go-github/github"
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

	mux := http.NewServeMux()
	// setup common handlers for local and deployed runs
	mux.Handle("/", reposHandler(ghClient))
	mux.Handle("/fakeghserver/", http.StripPrefix("/fakeghserver", reposHandler(ghClient)))

	health := pjutil.NewHealth()
	health.ServeReady()

	logrus.Info("Start server")

	// setup done, actually start the server
	server := &http.Server{Addr: ":8888", Handler: mux}
	interrupts.ListenAndServe(server, 5*time.Second)
}

func reposHandler(ghc *fakegithub.FakeClient) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirect(w, r, ghc)
	})
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

// So far, supports APIs used by crier:
//type GitHubClient interface {
// 	BotName() (string, error) # /user
// 	CreateStatus(org, repo, ref string, s github.Status) error # fmt.Sprintf("/repos/%s/%s/statuses/%s", org, repo, SHA)
// 	ListIssueComments(org, repo string, number int) ([]github.IssueComment, error) # fmt.Sprintf("/repos/%s/%s/issues/%d/comments", org, repo, number)
// 	CreateComment(org, repo string, number int, comment string) error # fmt.Sprintf("/repos/%s/%s/issues/%d/comments", org, repo, number),
// 	DeleteComment(org, repo string, ID int) error # fmt.Sprintf("/repos/%s/%s/issues/comments/%d", org, repo, number),
// 	EditComment(org, repo string, ID int, comment string) error # fmt.Sprintf("/repos/%s/%s/issues/comments/%d", org, repo, number),
func redirect(w http.ResponseWriter, r *http.Request, ghc *fakegithub.FakeClient) error {
	logrus.Infof("Got request %+v", *r)
	var ok int
	// By default ok is 200
	ok = http.StatusOK
	var err error
	var msg string
	parts := strings.Split(r.URL.Path, "/")
	if strings.TrimSpace(parts[0]) == "" {
		parts = parts[1:]
	}
	switch {
	case len(parts) == 0:
		err = fmt.Errorf("{\"error\": \"API not supported\"}, %s, %s", r.URL.Path, r.Method)
	case parts[0] == "user":
		userData, err := ghc.BotUser()
		if err == nil {
			var content []byte
			content, err = json.Marshal(&userData)
			msg = string(content)
		}
	case parts[0] == "repos":
		switch {
		case len(parts) == 4 && parts[3] == "issues":
			// Create status is 201
			ok = http.StatusCreated
			data := github.Issue{}
			if err := unmarshal(w, r, &data); err != nil {
				return fmt.Errorf("failed processing payload: %v", err)
			}
			org, repo := parts[1], parts[2]
			var id int
			id, err = ghc.CreateIssue(org, repo, *data.Title, *data.Body, *data.Milestone.Number, nil, nil)
			msg = fmt.Sprintf("Issue %d created", id)
		case len(parts) == 5 && parts[3] == "statuses":
			if r.Method == http.MethodPost {
				ok = http.StatusCreated
				data := prowgh.Status{}
				if err := unmarshal(w, r, &data); err != nil {
					return fmt.Errorf("failed processing payload: %v", err)
				}
				org, repo, SHA := parts[1], parts[2], parts[4]
				err = ghc.CreateStatus(org, repo, SHA, data)
			} else if r.Method == http.MethodGet {
				org, repo, SHA := parts[1], parts[2], parts[4]
				var res []prowgh.Status
				res, err = ghc.ListStatuses(org, repo, SHA)
				if err == nil {
					var content []byte
					content, err = json.Marshal(res)
					if err == nil {
						msg = string(content)
					}
				}
			}
		case len(parts) == 6 && parts[3] == "issues" && parts[5] == "comments":
			org, repo, issueID := parts[1], parts[2], parts[4]
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
					data := prowgh.IssueComment{}
					if err := unmarshal(w, r, &data); err != nil {
						return fmt.Errorf("failed processing payload: %v", err)
					}
					err = ghc.CreateComment(org, repo, id, data.Body)
				} else {
					err = fmt.Errorf("{\"error\": \"API not supported\"}, %s, %s", r.URL.Path, r.Method)
				}
			}
		case len(parts) == 6 && parts[3] == "issues" && parts[4] == "comments":
			org, repo, commentID := parts[1], parts[2], parts[5]
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
		default:
			err = fmt.Errorf("{\"error\": \"API not supported\"}, %s, %s", r.URL.Path, r.Method)
		}
	default:
		err = fmt.Errorf("{\"error\": \"API not supported\"}, %s, %s", r.URL.Path, r.Method)
	}
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, err.Error())
		logrus.Info(err)
		return err
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(ok)
	fmt.Fprint(w, msg)
	logrus.Info("Succeeded with request: ", ok)
	return nil
}
