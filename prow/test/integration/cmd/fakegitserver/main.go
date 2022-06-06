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

// fakegitserver serves Git repositories over HTTP for integration tests.
package main

import (
	"flag"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/interrupts"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/pjutil"
	"k8s.io/test-infra/prow/test/integration/internal/fakegitserver"
)

type options struct {
	port              int
	gitBinary         string
	gitReposParentDir string
}

func (o *options) validate() error {
	return nil
}

// flagOptions defines default options.
func flagOptions() *options {
	o := &options{}
	flag.IntVar(&o.port, "port", 8888, "Port to listen on.")
	flag.StringVar(&o.gitBinary, "git-binary", "/usr/bin/git", "Path to the `git` binary.")
	flag.StringVar(&o.gitReposParentDir, "git-repos-parent-dir", "/git-repo", "Path to the parent folder containing all Git repos to serve over HTTP.")
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

	health := pjutil.NewHealth()
	health.ServeReady()

	r := mux.NewRouter()

	// Only send requests under the /repo/... path to git-http-backend. This way
	// we can have other paths (if necessary) to take in custom commands from
	// integration tests (e.g., "/admin/reset" to reset all repos back to their
	// original state).
	r.PathPrefix("/repo").Handler(fakegitserver.GitCGIHandler(o.gitBinary, o.gitReposParentDir))
	r.PathPrefix("/setup-repo").Handler(fakegitserver.SetupRepoHandler(o.gitReposParentDir))

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", o.port),
		Handler: r,
	}

	logrus.Info("Start server")
	interrupts.ListenAndServe(server, 5*time.Second)
}
