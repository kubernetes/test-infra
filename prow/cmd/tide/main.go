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
	"bytes"
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/git"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/tide"
)

type options struct {
	port int

	dryRun  bool
	runOnce bool
	deckURL string

	configPath    string
	jobConfigPath string
	cluster       string

	githubEndpoint  flagutil.Strings
	githubTokenFile string
}

func gatherOptions() options {
	o := options{
		githubEndpoint: flagutil.NewStrings("https://api.github.com"),
	}
	flag.IntVar(&o.port, "port", 8888, "Port to listen on.")

	flag.BoolVar(&o.dryRun, "dry-run", true, "Whether to mutate any real-world state.")
	flag.BoolVar(&o.runOnce, "run-once", false, "If true, run only once then quit.")
	flag.StringVar(&o.deckURL, "deck-url", "", "Deck URL for read-only access to the cluster.")

	flag.StringVar(&o.configPath, "config-path", "/etc/config/config.yaml", "Path to config.yaml.")
	flag.StringVar(&o.jobConfigPath, "job-config-path", "", "Path to prow job configs.")
	flag.StringVar(&o.cluster, "cluster", "", "Path to kube.Cluster YAML file. If empty, uses the local cluster.")

	flag.Var(&o.githubEndpoint, "github-endpoint", "GitHub's API endpoint.")
	flag.StringVar(&o.githubTokenFile, "github-token-file", "/etc/github/oauth", "Path to the file containing the GitHub OAuth token.")

	flag.Parse()
	return o
}

func main() {
	o := gatherOptions()

	logrus.SetFormatter(
		logrusutil.NewDefaultFieldsFormatter(nil, logrus.Fields{"component": "tide"}),
	)

	configAgent := &config.Agent{}
	if err := configAgent.Start(o.configPath, o.jobConfigPath); err != nil {
		logrus.WithError(err).Fatal("Error starting config agent.")
	}

	oauthSecretRaw, err := ioutil.ReadFile(o.githubTokenFile)
	if err != nil {
		logrus.WithError(err).Fatalf("Could not read oauth secret file.")
	}
	oauthSecret := string(bytes.TrimSpace(oauthSecretRaw))

	for _, ep := range o.githubEndpoint.Strings() {
		_, err = url.Parse(ep)
		if err != nil {
			logrus.WithError(err).Fatalf("Invalid --endpoint URL %q.", ep)
		}
	}

	var ghcSync, ghcStatus *github.Client
	var kc *kube.Client
	if o.dryRun {
		ghcSync = github.NewDryRunClient(oauthSecret, o.githubEndpoint.Strings()...)
		ghcStatus = github.NewDryRunClient(oauthSecret, o.githubEndpoint.Strings()...)
		kc = kube.NewFakeClient(o.deckURL)
	} else {
		ghcSync = github.NewClient(oauthSecret, o.githubEndpoint.Strings()...)
		ghcStatus = github.NewClient(oauthSecret, o.githubEndpoint.Strings()...)
		if o.cluster == "" {
			kc, err = kube.NewClientInCluster(configAgent.Config().ProwJobNamespace)
			if err != nil {
				logrus.WithError(err).Fatal("Error getting kube client.")
			}
		} else {
			kc, err = kube.NewClientFromFile(o.cluster, configAgent.Config().ProwJobNamespace)
			if err != nil {
				logrus.WithError(err).Fatal("Error getting kube client.")
			}
		}
	}
	// The sync loop should be allowed more tokens than the status loop because
	// it has to list all PRs in the pool every loop while the status loop only
	// has to list changed PRs every loop.
	// The sync loop should have a much lower burst allowance than the status
	// loop which may need to update many statuses upon restarting Tide after
	// changing the context format or starting Tide on a new repo.
	ghcSync.Throttle(800, 20)
	ghcStatus.Throttle(400, 200)

	gc, err := git.NewClient()
	if err != nil {
		logrus.WithError(err).Fatal("Error getting git client.")
	}
	defer gc.Clean()
	// Get the bot's name in order to set credentials for the git client.
	botName, err := ghcSync.BotName()
	if err != nil {
		logrus.WithError(err).Fatal("Error getting bot name.")
	}
	gc.SetCredentials(botName, oauthSecret)

	c := tide.NewController(ghcSync, ghcStatus, kc, configAgent, gc, nil)
	defer c.Shutdown()

	server := &http.Server{Addr: ":" + strconv.Itoa(o.port), Handler: c}

	start := time.Now()
	sync(c)
	if o.runOnce {
		return
	}
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		for {
			select {
			case <-time.After(time.Until(start.Add(configAgent.Config().Tide.SyncPeriod))):
				start = time.Now()
				sync(c)
			case <-sig:
				logrus.Info("Tide is shutting down...")
				// Shutdown the http server with a 10s timeout then return to execute
				// defered c.Shutdown()
				ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
				defer cancel() // frees ctx resources
				server.Shutdown(ctx)
				return
			}
		}
	}()
	logrus.WithError(server.ListenAndServe()).Warn("Tide HTTP server stopped.")
}

func sync(c *tide.Controller) {
	start := time.Now()
	if err := c.Sync(); err != nil {
		logrus.WithError(err).Error("Error syncing.")
	}
	logrus.WithField("duration", fmt.Sprintf("%v", time.Since(start))).Info("Synced")
}
