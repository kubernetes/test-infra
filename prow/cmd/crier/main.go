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

package main

import (
	"context"
	"errors"
	"flag"
	"os"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/pjutil/pprof"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/config/secret"
	"k8s.io/test-infra/prow/crier"
	gcsreporter "k8s.io/test-infra/prow/crier/reporters/gcs"
	k8sgcsreporter "k8s.io/test-infra/prow/crier/reporters/gcs/kubernetes"
	gerritreporter "k8s.io/test-infra/prow/crier/reporters/gerrit"
	githubreporter "k8s.io/test-infra/prow/crier/reporters/github"
	pubsubreporter "k8s.io/test-infra/prow/crier/reporters/pubsub"
	slackreporter "k8s.io/test-infra/prow/crier/reporters/slack"
	prowflagutil "k8s.io/test-infra/prow/flagutil"
	configflagutil "k8s.io/test-infra/prow/flagutil/config"
	"k8s.io/test-infra/prow/interrupts"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/metrics"
	slackclient "k8s.io/test-infra/prow/slack"
)

type options struct {
	client           prowflagutil.KubernetesOptions
	cookiefilePath   string
	github           prowflagutil.GitHubOptions
	githubEnablement prowflagutil.GitHubEnablementOptions
	gerrit           prowflagutil.GerritOptions

	config configflagutil.ConfigOptions

	gerritWorkers         int
	pubsubWorkers         int
	githubWorkers         int
	slackWorkers          int
	blobStorageWorkers    int
	k8sBlobStorageWorkers int

	slackTokenFile            string
	additionalSlackTokenFiles slackclient.HostsFlag

	storage prowflagutil.StorageClientOptions

	instrumentationOptions prowflagutil.InstrumentationOptions

	k8sReportFraction float64

	dryrun      bool
	reportAgent string
}

func (o *options) validate() error {
	if o.gerritWorkers+o.pubsubWorkers+o.githubWorkers+o.slackWorkers+o.blobStorageWorkers+o.k8sBlobStorageWorkers <= 0 {
		return errors.New("crier need to have at least one report worker to start")
	}

	if o.k8sReportFraction < 0 || o.k8sReportFraction > 1 {
		return errors.New("--kubernetes-report-fraction must be a float between 0 and 1")
	}

	if o.gerritWorkers > 0 {
		if o.cookiefilePath == "" {
			logrus.Info("--cookiefile is not set, using anonymous authentication")
		}
		if err := o.gerrit.Validate(o.dryrun); err != nil {
			return err
		}
	}

	if o.githubWorkers > 0 {
		if err := o.github.Validate(o.dryrun); err != nil {
			return err
		}
	}

	if o.slackWorkers > 0 {
		if o.slackTokenFile == "" && len(o.additionalSlackTokenFiles) == 0 {
			return errors.New("one of --slack-token-file or --additional-slack-token-files must be set")
		}
	}

	for _, opt := range []interface{ Validate(bool) error }{&o.client, &o.githubEnablement, &o.config} {
		if err := opt.Validate(o.dryrun); err != nil {
			return err
		}
	}

	return nil
}

func (o *options) parseArgs(fs *flag.FlagSet, args []string) error {
	fs.StringVar(&o.cookiefilePath, "cookiefile", "", "Path to git http.cookiefile, leave empty for anonymous")
	fs.IntVar(&o.gerritWorkers, "gerrit-workers", 0, "Number of gerrit report workers (0 means disabled)")
	fs.IntVar(&o.pubsubWorkers, "pubsub-workers", 0, "Number of pubsub report workers (0 means disabled)")
	fs.IntVar(&o.githubWorkers, "github-workers", 0, "Number of github report workers (0 means disabled)")
	fs.IntVar(&o.slackWorkers, "slack-workers", 0, "Number of Slack report workers (0 means disabled)")
	fs.Var(&o.additionalSlackTokenFiles, "additional-slack-token-files", "Map of additional slack token files. example: --additional-slack-token-files=foo=/etc/foo-slack-tokens/token, repeat flag for each host")
	fs.IntVar(&o.blobStorageWorkers, "blob-storage-workers", 0, "Number of blob storage report workers (0 means disabled)")
	fs.IntVar(&o.k8sBlobStorageWorkers, "kubernetes-blob-storage-workers", 0, "Number of Kubernetes-specific blob storage report workers (0 means disabled)")
	fs.Float64Var(&o.k8sReportFraction, "kubernetes-report-fraction", 1.0, "Approximate portion of jobs to report pod information for, if kubernetes-blob-storage-workers are enabled (0 - > none, 1.0 -> all)")
	fs.StringVar(&o.slackTokenFile, "slack-token-file", "", "Path to a Slack token file")
	fs.StringVar(&o.reportAgent, "report-agent", "", "Only report specified agent - empty means report to all agents (effective for github and Slack only)")

	// TODO(krzyzacy): implement dryrun for gerrit/pubsub
	fs.BoolVar(&o.dryrun, "dry-run", false, "Run in dry-run mode, not doing actual report (effective for github and Slack only)")

	o.config.AddFlags(fs)
	o.github.AddFlags(fs)
	o.gerrit.AddFlags(fs)
	o.client.AddFlags(fs)
	o.storage.AddFlags(fs)
	o.instrumentationOptions.AddFlags(fs)
	o.githubEnablement.AddFlags(fs)

	fs.Parse(args)

	return o.validate()
}

func parseOptions() options {
	var o options

	if err := o.parseArgs(flag.CommandLine, os.Args[1:]); err != nil {
		logrus.WithError(err).Fatal("Invalid flag options")
	}

	return o
}

func main() {
	logrusutil.ComponentInit()

	o := parseOptions()

	pprof.Instrument(o.instrumentationOptions)

	configAgent, err := o.config.ConfigAgent()
	if err != nil {
		logrus.WithError(err).Fatal("Error starting config agent.")
	}
	cfg := configAgent.Config
	o.client.SetDisabledClusters(sets.New[string](cfg().DisabledClusters...))

	restCfg, err := o.client.InfrastructureClusterConfig(o.dryrun)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to get kubeconfig")
	}
	mgr, err := manager.New(restCfg, manager.Options{
		Namespace:          cfg().ProwJobNamespace,
		MetricsBindAddress: "0",
	})
	if err != nil {
		logrus.WithError(err).Fatal("failed to create manager")
	}

	// The watch apimachinery doesn't support restarts, so just exit the binary if a kubeconfig changes
	// to make the kubelet restart us.
	if err := o.client.AddKubeconfigChangeCallback(func() {
		logrus.Info("Kubeconfig changed, exiting to trigger a restart")
		interrupts.Terminate()
	}); err != nil {
		logrus.WithError(err).Fatal("Failed to register kubeconfig change callback")
	}

	var hasReporter bool
	if o.slackWorkers > 0 {
		if cfg().SlackReporterConfigs == nil {
			logrus.Fatal("slackreporter is enabled but has no config")
		}
		slackConfig := func(refs *prowapi.Refs) config.SlackReporter {
			return cfg().SlackReporterConfigs.GetSlackReporter(refs)
		}
		tokensMap := make(map[string]func() []byte)
		if o.slackTokenFile != "" {
			tokensMap[slackreporter.DefaultHostName] = secret.GetTokenGenerator(o.slackTokenFile)
			if err := secret.Add(o.slackTokenFile); err != nil {
				logrus.WithError(err).Fatal("could not read slack token")
			}
		}
		hasReporter = true
		for host, additionalTokenFile := range o.additionalSlackTokenFiles {
			tokensMap[host] = secret.GetTokenGenerator(additionalTokenFile)
			if err := secret.Add(additionalTokenFile); err != nil {
				logrus.WithError(err).Fatal("could not read slack token")
			}
		}
		slackReporter := slackreporter.New(slackConfig, o.dryrun, tokensMap)
		if err := crier.New(mgr, slackReporter, o.slackWorkers, o.githubEnablement.EnablementChecker()); err != nil {
			logrus.WithError(err).Fatal("failed to construct slack reporter controller")
		}
	}

	if o.gerritWorkers > 0 {
		orgRepoConfigGetter := func() *config.GerritOrgRepoConfigs {
			return cfg().Gerrit.OrgReposConfig
		}
		gerritReporter, err := gerritreporter.NewReporter(orgRepoConfigGetter, o.cookiefilePath, mgr.GetClient(), o.gerrit.MaxQPS, o.gerrit.MaxBurst)
		if err != nil {
			logrus.WithError(err).Fatal("Error starting gerrit reporter")
		}

		hasReporter = true
		if err := crier.New(mgr, gerritReporter, o.gerritWorkers, o.githubEnablement.EnablementChecker()); err != nil {
			logrus.WithError(err).Fatal("failed to construct gerrit reporter controller")
		}
	}

	if o.pubsubWorkers > 0 {
		hasReporter = true
		if err := crier.New(mgr, pubsubreporter.NewReporter(cfg), o.pubsubWorkers, o.githubEnablement.EnablementChecker()); err != nil {
			logrus.WithError(err).Fatal("failed to construct pubsub reporter controller")
		}
	}

	if o.githubWorkers > 0 {
		if o.github.TokenPath != "" {
			if err := secret.Add(o.github.TokenPath); err != nil {
				logrus.WithError(err).Fatal("Error reading GitHub credentials")
			}
		}

		githubClient, err := o.github.GitHubClient(o.dryrun)
		if err != nil {
			logrus.WithError(err).Fatal("Error getting GitHub client.")
		}

		hasReporter = true
		githubReporter := githubreporter.NewReporter(githubClient, cfg, prowapi.ProwJobAgent(o.reportAgent), mgr.GetCache())
		if err := crier.New(mgr, githubReporter, o.githubWorkers, o.githubEnablement.EnablementChecker()); err != nil {
			logrus.WithError(err).Fatal("failed to construct github reporter controller")
		}
	}

	if o.blobStorageWorkers > 0 || o.k8sBlobStorageWorkers > 0 {
		opener, err := o.storage.StorageClient(context.Background())
		if err != nil {
			logrus.WithError(err).Fatal("Error creating opener")
		}

		hasReporter = true
		if o.blobStorageWorkers > 0 {
			if err := crier.New(mgr, gcsreporter.New(cfg, opener, o.dryrun), o.blobStorageWorkers, o.githubEnablement.EnablementChecker()); err != nil {
				logrus.WithError(err).Fatal("failed to construct gcsreporter controller")
			}
		}

		if o.k8sBlobStorageWorkers > 0 {
			coreClients, err := o.client.BuildClusterCoreV1Clients(o.dryrun)
			if err != nil {
				logrus.WithError(err).Fatal("Error building pod client sets for Kubernetes GCS workers")
			}

			k8sGcsReporter := k8sgcsreporter.New(cfg, opener, k8sgcsreporter.NewK8sResourceGetter(coreClients), float32(o.k8sReportFraction), o.dryrun)
			if err := crier.New(mgr, k8sGcsReporter, o.k8sBlobStorageWorkers, o.githubEnablement.EnablementChecker()); err != nil {
				logrus.WithError(err).Fatal("failed to construct k8sgcsreporter controller")
			}
		}
	}

	if !hasReporter {
		logrus.Fatalf("should have at least one controller to start crier.")
	}

	// Push metrics to the configured prometheus pushgateway endpoint or serve them
	metrics.ExposeMetrics("crier", cfg().PushGateway, o.instrumentationOptions.MetricsPort)

	interrupts.Run(func(ctx context.Context) {
		if err := mgr.Start(ctx); err != nil {
			logrus.WithError(err).Fatal("Controller manager exited with error.")
		}
	})
	interrupts.WaitForGracefulShutdown()
	logrus.Info("Ended gracefully")
}
