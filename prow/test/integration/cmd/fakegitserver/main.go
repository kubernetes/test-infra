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
	"os"
	"sync"
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
	portHttps         int
	gitBinary         string
	gitReposParentDir string
	cert              string
	key               string
}

func (o *options) validate() error {
	return nil
}

// flagOptions defines default options.
func flagOptions() *options {
	o := &options{}
	flag.IntVar(&o.port, "port", 8888, "Port to listen on.")
	flag.IntVar(&o.portHttps, "port-https", 4443, "Port to listen on for HTTPS traffic.")
	flag.StringVar(&o.gitBinary, "git-binary", "/usr/bin/git", "Path to the `git` binary.")
	flag.StringVar(&o.gitReposParentDir, "git-repos-parent-dir", "/git-repo", "Path to the parent folder containing all Git repos to serve over HTTP.")
	flag.StringVar(&o.cert, "cert", "", "Path to the server cert file for HTTPS.")
	flag.StringVar(&o.key, "key", "", "Path to the server key file for HTTPS.")
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
	// Set up repo might modify global git config, need to lock it to avoid
	// errors caused by concurrent modifications.
	var lock sync.Mutex
	r.PathPrefix("/setup-repo").Handler(fakegitserver.SetupRepoHandler(o.gitReposParentDir, &lock))

	if err := os.MkdirAll(o.gitReposParentDir, os.ModePerm); err != nil {
		logrus.Fatalf("could not create directory %q", o.gitReposParentDir)
	}

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", o.port),
		Handler: r,
	}

	// Serve HTTPS traffic.
	if o.cert != "" && o.key != "" {
		serverHTTPS := &http.Server{
			Addr:    fmt.Sprintf(":%d", o.portHttps),
			Handler: r,
		}
		logrus.Infof("Starting HTTPS server on port %d", o.portHttps)
		interrupts.ListenAndServeTLS(serverHTTPS, o.cert, o.key, 5*time.Second)
	}

	// Serve HTTP traffic.
	logrus.Infof("Starting HTTP server on port %d", o.port)
	interrupts.ListenAndServe(server, 5*time.Second)
}
