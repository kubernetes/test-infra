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
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/pkg/flagutil"
	"k8s.io/test-infra/pkg/io"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/config/secret"
	prowflagutil "k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/metrics"
	"k8s.io/test-infra/prow/pjutil"
	"k8s.io/test-infra/prow/tide"
)

type options struct {
	port int

	configPath    string
	jobConfigPath string

	syncThrottle   int
	statusThrottle int

	dryRun     bool
	runOnce    bool
	kubernetes prowflagutil.ExperimentalKubernetesOptions
	github     prowflagutil.GitHubOptions

	maxRecordsPerPool int
	// The following are used for reading/writing to GCS.
	gcsCredentialsFile string
	// historyURI where Tide should store its action history.
	// Can be a /local/path or gs://path/to/object.
	// GCS writes will use the bucket's default acl for new objects. Ensure both that
	// a) the gcs credentials can write to this bucket
	// b) the default acls do not expose any private info
	historyURI string

	// statusURI where Tide store status update state.
	// Can be a /local/path or gs://path/to/object.
	// GCS writes will use the bucket's default acl for new objects. Ensure both that
	// a) the gcs credentials can write to this bucket
	// b) the default acls do not expose any private info
	statusURI string
}

func (o *options) Validate() error {
	for _, group := range []flagutil.OptionGroup{&o.kubernetes, &o.github} {
		if err := group.Validate(o.dryRun); err != nil {
			return err
		}
	}

	return nil
}

func gatherOptions() options {
	o := options{}
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	fs.IntVar(&o.port, "port", 8888, "Port to listen on.")
	fs.StringVar(&o.configPath, "config-path", "", "Path to config.yaml.")
	fs.StringVar(&o.jobConfigPath, "job-config-path", "", "Path to prow job configs.")
	fs.BoolVar(&o.dryRun, "dry-run", true, "Whether to mutate any real-world state.")
	fs.BoolVar(&o.runOnce, "run-once", false, "If true, run only once then quit.")
	for _, group := range []flagutil.OptionGroup{&o.kubernetes, &o.github} {
		group.AddFlags(fs)
	}
	fs.IntVar(&o.syncThrottle, "sync-hourly-tokens", 800, "The maximum number of tokens per hour to be used by the sync controller.")
	fs.IntVar(&o.statusThrottle, "status-hourly-tokens", 400, "The maximum number of tokens per hour to be used by the status controller.")

	fs.IntVar(&o.maxRecordsPerPool, "max-records-per-pool", 1000, "The maximum number of history records stored for an individual Tide pool.")
	fs.StringVar(&o.gcsCredentialsFile, "gcs-credentials-file", "", "File where Google Cloud authentication credentials are stored. Required for GCS writes.")
	fs.StringVar(&o.historyURI, "history-uri", "", "The /local/path or gs://path/to/object to store tide action history. GCS writes will use the default object ACL for the bucket")
	fs.StringVar(&o.statusURI, "status-path", "", "The /local/path or gs://path/to/object to store status controller state. GCS writes will use the default object ACL for the bucket.")

	fs.Parse(os.Args[1:])
	o.configPath = config.ConfigPath(o.configPath)
	return o
}

func main() {
	logrus.SetFormatter(
		logrusutil.NewDefaultFieldsFormatter(nil, logrus.Fields{"component": "tide"}),
	)

	pjutil.ServePProf()

	o := gatherOptions()
	if err := o.Validate(); err != nil {
		logrus.Fatalf("Invalid options: %v", err)
	}

	opener, err := io.NewOpener(context.Background(), o.gcsCredentialsFile)
	if err != nil {
		entry := logrus.WithError(err)
		if p := o.gcsCredentialsFile; p != "" {
			entry = entry.WithField("gcs-credentials-file", p)
		}
		entry.Fatal("Cannot create opener")
	}

	configAgent := &config.Agent{}
	if err := configAgent.Start(o.configPath, o.jobConfigPath); err != nil {
		logrus.WithError(err).Fatal("Error starting config agent.")
	}
	cfg := configAgent.Config

	secretAgent := &secret.Agent{}
	if err := secretAgent.Start([]string{o.github.TokenPath}); err != nil {
		logrus.WithError(err).Fatal("Error starting secrets agent.")
	}

	githubSync, err := o.github.GitHubClientWithLogFields(secretAgent, o.dryRun, logrus.Fields{"controller": "sync"})
	if err != nil {
		logrus.WithError(err).Fatal("Error getting GitHub client for sync.")
	}

	githubStatus, err := o.github.GitHubClientWithLogFields(secretAgent, o.dryRun, logrus.Fields{"controller": "status-update"})
	if err != nil {
		logrus.WithError(err).Fatal("Error getting GitHub client for status.")
	}

	// The sync loop should be allowed more tokens than the status loop because
	// it has to list all PRs in the pool every loop while the status loop only
	// has to list changed PRs every loop.
	// The sync loop should have a much lower burst allowance than the status
	// loop which may need to update many statuses upon restarting Tide after
	// changing the context format or starting Tide on a new repo.
	githubSync.Throttle(o.syncThrottle, 3*tokensPerIteration(o.syncThrottle, cfg().Tide.SyncPeriod))
	githubStatus.Throttle(o.statusThrottle, o.statusThrottle/2)

	gitClient, err := o.github.GitClient(secretAgent, o.dryRun)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting Git client.")
	}
	defer gitClient.Clean()

	kubeClient, err := o.kubernetes.ProwJobClient(cfg().ProwJobNamespace, o.dryRun)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting Kubernetes client.")
	}

	c, err := tide.NewController(githubSync, githubStatus, kubeClient, cfg, gitClient, o.maxRecordsPerPool, opener, o.historyURI, o.statusURI, nil)
	if err != nil {
		logrus.WithError(err).Fatal("Error creating Tide controller.")
	}
	defer c.Shutdown()
	http.Handle("/", c)
	http.Handle("/history", c.History)
	server := &http.Server{Addr: ":" + strconv.Itoa(o.port)}

	// Push metrics to the configured prometheus pushgateway endpoint.
	pushGateway := cfg().PushGateway
	if pushGateway.Endpoint != "" {
		go metrics.PushMetrics("tide", pushGateway.Endpoint, pushGateway.Interval)
	}

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
			case <-time.After(time.Until(start.Add(cfg().Tide.SyncPeriod))):
				start = time.Now()
				sync(c)
			case <-sig:
				logrus.Info("Tide is shutting down...")
				// Shutdown the http server with a 10s timeout then return to execute
				// deferred c.Shutdown()
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
	if err := c.Sync(); err != nil {
		logrus.WithError(err).Error("Error syncing.")
	}
}

func tokensPerIteration(hourlyTokens int, iterPeriod time.Duration) int {
	tokenRate := float64(hourlyTokens) / float64(time.Hour)
	return int(tokenRate * float64(iterPeriod))
}
