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
	"errors"
	"flag"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/pjutil/pprof"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"k8s.io/test-infra/pkg/flagutil"
	prowflagutil "k8s.io/test-infra/prow/flagutil"
	configflagutil "k8s.io/test-infra/prow/flagutil/config"
	"k8s.io/test-infra/prow/interrupts"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/metrics"
	"k8s.io/test-infra/prow/tide"
)

const (
	githubProviderName = "github"
	gerritProviderName = "gerrit"
)

type options struct {
	port int

	config configflagutil.ConfigOptions

	syncThrottle   int
	statusThrottle int

	dryRun                 bool
	runOnce                bool
	kubernetes             prowflagutil.KubernetesOptions
	github                 prowflagutil.GitHubOptions
	gerrit                 prowflagutil.GerritOptions
	storage                prowflagutil.StorageClientOptions
	instrumentationOptions prowflagutil.InstrumentationOptions
	controllerManager      prowflagutil.ControllerManagerOptions

	maxRecordsPerPool int
	// historyURI where Tide should store its action history.
	// Can be /local/path, gs://path/to/object or s3://path/to/object.
	// GCS writes will use the bucket's default acl for new objects. Ensure both that
	// a) the gcs credentials can write to this bucket
	// b) the default acls do not expose any private info
	historyURI string

	// statusURI where Tide store status update state.
	// Can be a /local/path, gs://path/to/object or s3://path/to/object.
	// GCS writes will use the bucket's default acl for new objects. Ensure both that
	// a) the gcs credentials can write to this bucket
	// b) the default acls do not expose any private info
	statusURI string

	// providerName is
	providerName string

	// Gerrit-related options
	cookiefilePath string
}

func (o *options) Validate() error {
	for _, group := range []flagutil.OptionGroup{&o.kubernetes, &o.storage, &o.config, &o.controllerManager} {
		if err := group.Validate(o.dryRun); err != nil {
			return err
		}
	}
	if o.providerName != "" && !sets.NewString(githubProviderName, gerritProviderName).Has(o.providerName) {
		return errors.New("--provider should be github or gerrit")
	}
	var providerFlagGroup flagutil.OptionGroup = &o.github
	if o.providerName == gerritProviderName {
		providerFlagGroup = &o.gerrit
	}
	if err := providerFlagGroup.Validate(o.dryRun); err != nil {
		return err
	}
	return nil
}

func gatherOptions(fs *flag.FlagSet, args ...string) options {
	var o options
	fs.IntVar(&o.port, "port", 8888, "Port to listen on.")
	fs.BoolVar(&o.dryRun, "dry-run", true, "Whether to mutate any real-world state.")
	fs.BoolVar(&o.runOnce, "run-once", false, "If true, run only once then quit.")
	o.github.AddCustomizedFlags(fs, prowflagutil.DisableThrottlerOptions())
	for _, group := range []flagutil.OptionGroup{&o.kubernetes, &o.storage, &o.instrumentationOptions, &o.config, &o.gerrit} {
		group.AddFlags(fs)
	}
	fs.IntVar(&o.syncThrottle, "sync-hourly-tokens", 800, "The maximum number of tokens per hour to be used by the sync controller.")
	fs.IntVar(&o.statusThrottle, "status-hourly-tokens", 400, "The maximum number of tokens per hour to be used by the status controller.")
	fs.IntVar(&o.maxRecordsPerPool, "max-records-per-pool", 1000, "The maximum number of history records stored for an individual Tide pool.")
	fs.StringVar(&o.historyURI, "history-uri", "", "The /local/path,gs://path/to/object or s3://path/to/object to store tide action history. GCS writes will use the default object ACL for the bucket")
	fs.StringVar(&o.statusURI, "status-path", "", "The /local/path, gs://path/to/object or s3://path/to/object to store status controller state. GCS writes will use the default object ACL for the bucket.")
	// Gerrit-related flags
	fs.StringVar(&o.cookiefilePath, "cookiefile", "", "Path to git http.cookiefile; leave empty for anonymous access or if you are using GitHub")

	fs.StringVar(&o.providerName, "provider", "", "The source code provider, only supported providers are github and gerrit, this should be set only when both GitHub and Gerrit configs are set for tide. By default provider is auto-detected as github if `tide.queries` is set, and gerrit if `tide.gerrit` is set.")
	o.controllerManager.TimeoutListingProwJobsDefault = 30 * time.Second
	o.controllerManager.AddFlags(fs)
	fs.Parse(args)
	return o
}

func main() {
	logrusutil.ComponentInit()

	defer interrupts.WaitForGracefulShutdown()

	o := gatherOptions(flag.NewFlagSet(os.Args[0], flag.ExitOnError), os.Args[1:]...)
	if err := o.Validate(); err != nil {
		logrus.WithError(err).Fatal("Invalid options")
	}

	pprof.Instrument(o.instrumentationOptions)

	opener, err := o.storage.StorageClient(context.Background())
	if err != nil {
		logrus.WithError(err).Fatal("Cannot create opener")
	}

	configAgent, err := o.config.ConfigAgent()
	if err != nil {
		logrus.WithError(err).Fatal("Error starting config agent.")
	}
	cfg := configAgent.Config

	kubeCfg, err := o.kubernetes.InfrastructureClusterConfig(o.dryRun)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting kubeconfig.")
	}
	// Do not activate leader election here, as we do not use the `mgr` to control the lifecylcle of our cotrollers,
	// this would just be a no-op.
	mgr, err := manager.New(kubeCfg, manager.Options{Namespace: cfg().ProwJobNamespace, MetricsBindAddress: "0"})
	if err != nil {
		logrus.WithError(err).Fatal("Error constructing mgr.")
	}

	if cfg().Tide.Gerrit != nil && cfg().Tide.Queries.QueryMap() != nil && o.providerName == "" {
		logrus.Fatal("Both github and gerrit are configured in tide config but provider is not set.")
	}

	var c *tide.Controller
	gitClient, err := o.github.GitClientFactory(o.cookiefilePath, &o.config.InRepoConfigCacheDirBase, o.dryRun, false)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting Git client.")
	}
	provider := provider(o.providerName, cfg().Tide)
	switch provider {
	case githubProviderName:
		githubSync, err := o.github.GitHubClientWithLogFields(o.dryRun, logrus.Fields{"controller": "sync"})
		if err != nil {
			logrus.WithError(err).Fatal("Error getting GitHub client for sync.")
		}

		githubStatus, err := o.github.GitHubClientWithLogFields(o.dryRun, logrus.Fields{"controller": "status-update"})
		if err != nil {
			logrus.WithError(err).Fatal("Error getting GitHub client for status.")
		}

		// The sync loop should be allowed more tokens than the status loop because
		// it has to list all PRs in the pool every loop while the status loop only
		// has to list changed PRs every loop.
		// The sync loop should have a much lower burst allowance than the status
		// loop which may need to update many statuses upon restarting Tide after
		// changing the context format or starting Tide on a new repo.
		githubSync.Throttle(o.syncThrottle, 3*tokensPerIteration(o.syncThrottle, cfg().Tide.SyncPeriod.Duration))
		githubStatus.Throttle(o.statusThrottle, o.statusThrottle/2)

		c, err = tide.NewController(
			githubSync,
			githubStatus,
			mgr,
			cfg,
			gitClient,
			o.maxRecordsPerPool,
			opener,
			o.historyURI,
			o.statusURI,
			nil,
			o.github.AppPrivateKeyPath != "",
		)
		if err != nil {
			logrus.WithError(err).Fatal("Error creating Tide controller.")
		}
	case gerritProviderName:
		c, err = tide.NewGerritController(
			mgr,
			configAgent,
			gitClient,
			o.maxRecordsPerPool,
			opener,
			o.historyURI,
			o.statusURI,
			nil,
			o.config,
			o.cookiefilePath,
			o.gerrit.MaxQPS,
			o.gerrit.MaxBurst,
		)
		if err != nil {
			logrus.WithError(err).Fatal("Error creating Tide controller.")
		}
	default:
		logrus.Fatalf("Unsupported provider type '%s', this should not happen", provider)
	}

	interrupts.Run(func(ctx context.Context) {
		if err := mgr.Start(ctx); err != nil {
			logrus.WithError(err).Fatal("Mgr failed.")
		}
		logrus.Info("Mgr finished gracefully.")
	})

	mgrSyncCtx, mgrSyncCtxCancel := context.WithTimeout(context.Background(), o.controllerManager.TimeoutListingProwJobs)
	defer mgrSyncCtxCancel()
	if synced := mgr.GetCache().WaitForCacheSync(mgrSyncCtx); !synced {
		logrus.Fatal("Timed out waiting for cachesync")
	}

	interrupts.OnInterrupt(func() {
		c.Shutdown()
		if err := gitClient.Clean(); err != nil {
			logrus.WithError(err).Error("Could not clean up git client cache.")
		}
	})

	// Deck consumes these endpoints
	controllerMux := http.NewServeMux()
	controllerMux.Handle("/", c)
	controllerMux.Handle("/history", c.History())
	server := &http.Server{Addr: ":" + strconv.Itoa(o.port), Handler: controllerMux}

	// Push metrics to the configured prometheus pushgateway endpoint or serve them
	metrics.ExposeMetrics("tide", cfg().PushGateway, o.instrumentationOptions.MetricsPort)

	start := time.Now()
	sync(c)
	if o.runOnce {
		return
	}

	// serve data
	interrupts.ListenAndServe(server, 10*time.Second)

	// run the controller, but only after one sync period expires after our first run
	time.Sleep(time.Until(start.Add(cfg().Tide.SyncPeriod.Duration)))
	interrupts.Tick(func() {
		sync(c)
	}, func() time.Duration {
		return cfg().Tide.SyncPeriod.Duration
	})
}

func sync(c *tide.Controller) {
	if err := c.Sync(); err != nil {
		logrus.WithError(err).Error("Error syncing.")
	}
}

func provider(wantProvider string, tideConfig config.Tide) string {
	if wantProvider != "" {
		if !sets.NewString(githubProviderName, gerritProviderName).Has(wantProvider) {
			return ""
		}
		return wantProvider
	}
	// Default to GitHub if GitHub queries are configured
	if len([]config.TideQuery(tideConfig.Queries)) > 0 {
		return githubProviderName
	}
	if tideConfig.Gerrit != nil && len([]config.GerritOrgRepoConfig(tideConfig.Gerrit.Queries)) > 0 {
		return gerritProviderName
	}
	// When nothing is configured, don't fail tide. Assuming
	return githubProviderName
}

func tokensPerIteration(hourlyTokens int, iterPeriod time.Duration) int {
	tokenRate := float64(hourlyTokens) / float64(time.Hour)
	return int(tokenRate * float64(iterPeriod))
}
