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

// inrepoconfig-server is a web server that serves purely other Prow components,
// it's responsible for caching and resolving inrepoconfig files and serving the
// resolved files via http requests from other components.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/NYTimes/gziphandler"
	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/pkg/flagutil"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/interrupts"
	"k8s.io/test-infra/prow/io"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/pjutil"

	"github.com/gorilla/schema"

	prowflagutil "k8s.io/test-infra/prow/flagutil"
	configflagutil "k8s.io/test-infra/prow/flagutil/config"
)

type options struct {
	port int

	github          prowflagutil.GitHubOptions
	config          configflagutil.ConfigOptions
	storage         prowflagutil.StorageClientOptions
	instrumentation prowflagutil.InstrumentationOptions

	// Gerrit-related options
	cookiefilePath string
	// cachefilePath is where cache is stored(potentially in GCS)
	cachefilePath string
}

func (o *options) Validate() error {
	for _, group := range []flagutil.OptionGroup{&o.github, &o.config, &o.storage, &o.instrumentation} {
		if err := group.Validate(false); err != nil {
			return err
		}
	}
	if o.cachefilePath == "" {
		return errors.New("cachefile must be provided")
	}
	return nil
}

func gatherOptions(fs *flag.FlagSet, args ...string) options {
	var o options
	fs.IntVar(&o.port, "port", 8888, "Port to listen on.")
	for _, group := range []flagutil.OptionGroup{&o.github, &o.config, &o.storage, &o.instrumentation} {
		group.AddFlags(fs)
	}
	// Gerrit-related flags
	fs.StringVar(&o.cookiefilePath, "cookiefile", "", "Path to git http.cookiefile; leave empty for anonymous access or if you are using GitHub")
	fs.StringVar(&o.cachefilePath, "cachefile", "", "Path to cachefile")
	fs.Parse(args)
	return o
}

type queryParams struct {
	Type     string   `schema:"type,required"`
	Org      string   `schema:"org,required"`
	Repo     string   `schema:"repo,required"`
	BaseSHA  string   `schema:"base_sha,required"`
	HeadSHAs []string `schema:"head_shas,required"`
}

type controller struct {
	cacheGetter *config.InRepoConfigCacheHandler

	decoder *schema.Decoder
	l       *logrus.Entry
}

// newController initializes a controller, constructing git clients for Gerrit
// and Github.
func newControler(cacheGetter *config.InRepoConfigCacheHandler) *controller {
	return &controller{
		decoder:     schema.NewDecoder(),
		l:           logrus.WithField("controller", "inrepoconfig-server"),
		cacheGetter: cacheGetter,
	}
}

func main() {
	if err := realMain(); err != nil {
		logrus.Fatal(err)
	}
}

// realMain is created instead of main, potentially can be used for unit testing
// purpose.
func realMain() error {
	logrusutil.ComponentInit()

	defer interrupts.WaitForGracefulShutdown()

	o := gatherOptions(flag.NewFlagSet(os.Args[0], flag.ExitOnError), os.Args[1:]...)
	if err := o.Validate(); err != nil {
		logrus.WithError(err).Fatal("Invalid options")
	}

	configAgent, err := o.config.ConfigAgent()
	if err != nil {
		logrus.WithError(err).Fatal("Error starting config agent.")
	}
	cfg := configAgent.Config

	// If we are provided credentials for Git hosts, use them. These credentials
	// hold per-host information in them so it's safe to set them globally.
	if o.cookiefilePath != "" {
		cmd := exec.Command("git", "config", "--global", "http.cookiefile", o.cookiefilePath)
		if err := cmd.Run(); err != nil {
			logrus.WithError(err).Fatal("unable to set cookiefile")
		}
	}
	gitClient, err := o.github.GitClientFactory(o.cookiefilePath, &o.config.InRepoConfigCacheDirBase, false)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting Git client.")
	}
	opener, err := io.NewOpener(context.Background(), o.storage.GCSCredentialsFile, o.storage.S3CredentialsFile)
	if err != nil {
		logrus.WithError(err).Fatal("Error creating io.Opener.")
	}
	cacheGetter, err := config.NewInRepoConfigCacheHandler(o.config.InRepoConfigCacheSize, configAgent, gitClient, o.config.InRepoConfigCacheCopies, o.cachefilePath, opener)
	if err != nil {
		logrus.WithError(err).Fatal("Error creating InRepoConfigCacheGetter.")
	}

	c := newControler(cacheGetter)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		msg := fmt.Sprintf("Unsupported path: %v", r.URL)
		http.Error(w, msg, http.StatusNotFound)
	})
	mux.Handle("/inrepoconfig", gziphandler.GzipHandler(handleInrepoconfig(c, cfg)))

	// setup done, actually start the server

	// signal to the world that we are healthy
	// this needs to be in a separate port as we don't start the
	// main server with the main mux until we're ready
	health := pjutil.NewHealthOnPort(o.instrumentation.HealthPort)
	// signal to the world that we're ready
	health.ServeReady()
	server := &http.Server{Addr: fmt.Sprintf(":%d", o.port), Handler: mux}
	interrupts.ListenAndServe(server, 5*time.Second)
	return nil
}

func handleInrepoconfig(c *controller, cfg func() *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		if err := r.ParseForm(); err != nil {
			respondWithError(c.l, w, fmt.Sprintf("Failed parse request form: %v", err), http.StatusBadRequest)
			return
		}
		var qps queryParams
		if err := c.decoder.Decode(&qps, r.PostForm); err != nil {
			respondWithError(c.l, w, fmt.Sprintf("Failed decoding request form into queryParams: %v", err), http.StatusBadRequest)
			return
		}

		baseSHAGetter := func() (string, error) { return qps.BaseSHA, nil }
		var headSHAGetters []func() (string, error)
		for _, hs := range qps.HeadSHAs {
			hs := hs
			headSHAGetters = append(headSHAGetters, func() (string, error) { return hs, nil })
		}

		var payload []byte
		var err error
		switch qps.Type {
		case "presubmits":
			presubmits, tmpErr := c.cacheGetter.GetPresubmits(qps.Org+"/"+qps.Repo, baseSHAGetter, headSHAGetters...)
			if tmpErr != nil {
				respondWithError(c.l, w, fmt.Sprintf("Failed getting presubmits: %v", err), http.StatusInternalServerError)
				return
			}
			payload, err = json.Marshal(presubmits)
		case "postsubmits":
			postsubmits, tmpErr := c.cacheGetter.GetPostsubmits(qps.Org+"/"+qps.Repo, baseSHAGetter, headSHAGetters...)
			if tmpErr != nil {
				respondWithError(c.l, w, fmt.Sprintf("Failed getting postsubmits: %v", err), http.StatusInternalServerError)
				return
			}
			payload, err = json.Marshal(postsubmits)
		}

		if err != nil {
			respondWithError(c.l, w, fmt.Sprintf("Failed to unmarshal inrepoconfig jobs: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, string(payload))
	}
}

func respondWithError(l *logrus.Entry, w http.ResponseWriter, msg string, code int) {
	l.WithFields(logrus.Fields{"message": msg, "code": code})
	http.Error(w, msg, code)
}
