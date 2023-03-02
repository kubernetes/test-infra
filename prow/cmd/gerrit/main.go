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
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/metrics"
	"k8s.io/test-infra/prow/pjutil/pprof"

	"k8s.io/test-infra/pkg/flagutil"
	prowflagutil "k8s.io/test-infra/prow/flagutil"
	configflagutil "k8s.io/test-infra/prow/flagutil/config"
	"k8s.io/test-infra/prow/gerrit/adapter"
	"k8s.io/test-infra/prow/interrupts"
	"k8s.io/test-infra/prow/logrusutil"
)

type options struct {
	cookiefilePath    string
	tokenPathOverride string
	config            configflagutil.ConfigOptions
	// lastSyncFallback is the path to sync the latest timestamp
	// Can be /local/path, gs://path/to/object or s3://path/to/object.
	lastSyncFallback       string
	dryRun                 bool
	kubernetes             prowflagutil.KubernetesOptions
	storage                prowflagutil.StorageClientOptions
	instrumentationOptions prowflagutil.InstrumentationOptions
	changeWorkerPoolSize   int
	retryAttempts          int
}

func (o *options) validate() error {
	if o.cookiefilePath != "" && o.tokenPathOverride != "" {
		return fmt.Errorf("only one of --cookiefile=%q --token-path=%q allowed, not both", o.cookiefilePath, o.tokenPathOverride)
	}
	if o.cookiefilePath == "" && o.tokenPathOverride == "" {
		logrus.Info("--cookiefile is not set, using anonymous authentication")
	}

	if err := o.config.Validate(o.dryRun); err != nil {
		return err
	}

	if o.lastSyncFallback == "" {
		return errors.New("--last-sync-fallback must be set")
	}

	if strings.HasPrefix(o.lastSyncFallback, "gs://") && !o.storage.HasGCSCredentials() {
		logrus.WithField("last-sync-fallback", o.lastSyncFallback).Info("--gcs-credentials-file unset, will try and access with a default service account")
	}
	if strings.HasPrefix(o.lastSyncFallback, "s3://") && !o.storage.HasS3Credentials() {
		logrus.WithField("last-sync-fallback", o.lastSyncFallback).Info("--s3-credentials-file unset, will try and access with auto-discovered credentials")
	}
	if o.changeWorkerPoolSize < 1 {
		return errors.New("change-worker-pool-size must be at least 1")
	}
	return nil
}

func gatherOptions(fs *flag.FlagSet, args ...string) options {
	var o options
	fs.StringVar(&o.cookiefilePath, "cookiefile", "", "Path to git http.cookiefile, leave empty for anonymous")
	fs.StringVar(&o.lastSyncFallback, "last-sync-fallback", "", "The /local/path, gs://path/to/object or s3://path/to/object to sync the latest timestamp")
	fs.BoolVar(&o.dryRun, "dry-run", false, "Run in dry-run mode, performing no modifying actions.")
	fs.StringVar(&o.tokenPathOverride, "token-path", "", "Force the use of the token in this path, use with gcloud auth print-access-token")
	fs.IntVar(&o.changeWorkerPoolSize, "change-worker-pool-size", 1, "Number of workers processing changes for each instance.")
	fs.IntVar(&o.retryAttempts, "retry-attempts", 10, "Number of times a job process will retry if failed")
	for _, group := range []flagutil.OptionGroup{&o.kubernetes, &o.storage, &o.instrumentationOptions, &o.config} {
		group.AddFlags(fs)
	}
	fs.Parse(args)
	return o
}

func main() {
	logrusutil.ComponentInit()

	o := gatherOptions(flag.NewFlagSet(os.Args[0], flag.ExitOnError), os.Args[1:]...)
	if err := o.validate(); err != nil {
		logrus.Fatalf("Invalid options: %v", err)
	}

	pprof.Instrument(o.instrumentationOptions)

	ca, err := o.config.ConfigAgent()
	if err != nil {
		logrus.WithError(err).Fatal("Error starting config agent.")
	}
	cfg := ca.Config

	// Expose Prometheus metrics
	metrics.ExposeMetrics("gerrit", cfg().PushGateway, o.instrumentationOptions.MetricsPort)

	prowJobClient, err := o.kubernetes.ProwJobClient(cfg().ProwJobNamespace, o.dryRun)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting kube client.")
	}

	ctx := context.Background() // TODO(fejta): use something better
	op, err := o.storage.StorageClient(ctx)
	if err != nil {
		logrus.WithError(err).Fatal("Error creating opener")
	}

	gitClient, err := (&prowflagutil.GitHubOptions{}).GitClientFactory(o.cookiefilePath, &o.config.InRepoConfigCacheDirBase, o.dryRun)
	if err != nil {
		logrus.WithError(err).Fatal("Error creating git client.")
	}
	cacheGetter, err := config.NewInRepoConfigCacheHandler(o.config.InRepoConfigCacheSize, ca, gitClient, o.config.InRepoConfigCacheCopies)
	if err != nil {
		logrus.WithError(err).Fatal("Error creating InRepoConfigCacheGetter.")
	}
	c := adapter.NewController(ctx, prowJobClient, op, ca, o.cookiefilePath, o.tokenPathOverride, o.lastSyncFallback, o.changeWorkerPoolSize, o.retryAttempts, cacheGetter)

	logrus.Infof("Starting gerrit fetcher")

	defer interrupts.WaitForGracefulShutdown()
	interrupts.Tick(func() {
		c.Sync()
	}, func() time.Duration {
		return cfg().Gerrit.TickInterval.Duration
	})
}
