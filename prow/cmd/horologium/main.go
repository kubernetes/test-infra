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
	"fmt"
	"os"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/pjutil/pprof"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/cron"
	"k8s.io/test-infra/prow/flagutil"
	prowflagutil "k8s.io/test-infra/prow/flagutil"
	configflagutil "k8s.io/test-infra/prow/flagutil/config"
	"k8s.io/test-infra/prow/interrupts"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/metrics"
	"k8s.io/test-infra/prow/pjutil"
)

const (
	defaultTickInterval = time.Minute
)

type options struct {
	config configflagutil.ConfigOptions

	kubernetes             flagutil.KubernetesOptions
	instrumentationOptions prowflagutil.InstrumentationOptions
	dryRun                 bool
}

func gatherOptions(fs *flag.FlagSet, args ...string) options {
	var o options

	fs.BoolVar(&o.dryRun, "dry-run", true, "Whether or not to make mutating API calls to Kubernetes.")
	o.config.AddFlags(fs)
	o.kubernetes.AddFlags(fs)
	o.instrumentationOptions.AddFlags(fs)

	fs.Parse(args)
	return o
}

func (o *options) Validate() error {
	if err := o.kubernetes.Validate(o.dryRun); err != nil {
		return err
	}

	if err := o.config.Validate(o.dryRun); err != nil {
		return errors.New("--config-path is required")
	}

	return nil
}

func main() {
	logrusutil.ComponentInit()

	o := gatherOptions(flag.NewFlagSet(os.Args[0], flag.ExitOnError), os.Args[1:]...)
	if err := o.Validate(); err != nil {
		logrus.WithError(err).Fatal("Invalid options")
	}

	defer interrupts.WaitForGracefulShutdown()

	pprof.Instrument(o.instrumentationOptions)

	configAgent, err := o.config.ConfigAgent()
	if err != nil {
		logrus.WithError(err).Fatal("Error starting config agent.")
	}

	cfg, err := o.kubernetes.InfrastructureClusterConfig(o.dryRun)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to get prowjob kubeconfig")
	}
	cluster, err := cluster.New(cfg, func(o *cluster.Options) { o.Namespace = configAgent.Config().ProwJobNamespace })
	if err != nil {
		logrus.WithError(err).Fatal("failed to construct prowjob client")
	}
	// Trigger cache creation for ProwJobs so the following cacheSync actually does something. If we don't
	// do this here, the first List request for ProwJobs will transiently trigger cache creation and sync,
	// which doesn't allow us to fail the binary if it doesn't work.
	if _, err := cluster.GetCache().GetInformer(interrupts.Context(), &prowapi.ProwJob{}); err != nil {
		logrus.WithError(err).Fatal("failed to get a prowjob informer")
	}
	interrupts.Run(func(ctx context.Context) {
		if err := cluster.Start(ctx); err != nil {
			logrus.WithError(err).Fatal("Cache failed to start")
		}
		logrus.Info("Cache finished gracefully.")
	})
	cacheSyncCtx, cacheSyncCancel := context.WithTimeout(context.Background(), time.Minute)
	defer cacheSyncCancel()
	if synced := cluster.GetCache().WaitForCacheSync(cacheSyncCtx); !synced {
		logrus.Fatal("Timed out waiting for cachesync")
	}

	// start a cron
	cr := cron.New()
	cr.Start()

	metrics.ExposeMetrics("horologium", configAgent.Config().PushGateway, o.instrumentationOptions.MetricsPort)

	tickInterval := defaultTickInterval
	if configAgent.Config().Horologium.TickInterval != nil {
		tickInterval = configAgent.Config().Horologium.TickInterval.Duration
	}
	interrupts.TickLiteral(func() {
		start := time.Now()
		if err := sync(cluster.GetClient(), configAgent.Config(), cr, start); err != nil {
			logrus.WithError(err).Error("Error syncing periodic jobs.")
		}
		logrus.WithField("duration", time.Since(start)).Info("Synced periodic jobs")
	}, tickInterval)
}

type cronClient interface {
	SyncConfig(cfg *config.Config) error
	QueuedJobs() []string
}

func sync(prowJobClient ctrlruntimeclient.Client, cfg *config.Config, cr cronClient, now time.Time) error {
	jobs := &prowapi.ProwJobList{}
	if err := prowJobClient.List(context.TODO(), jobs, ctrlruntimeclient.InNamespace(cfg.ProwJobNamespace)); err != nil {
		return fmt.Errorf("error listing prow jobs: %w", err)
	}
	latestJobs := pjutil.GetLatestProwJobs(jobs.Items, prowapi.PeriodicJob)

	if err := cr.SyncConfig(cfg); err != nil {
		logrus.WithError(err).Error("Error syncing cron jobs.")
	}

	cronTriggers := sets.NewString()
	for _, job := range cr.QueuedJobs() {
		cronTriggers.Insert(job)
	}

	var errs []error
	for _, p := range cfg.Periodics {
		j, previousFound := latestJobs[p.Name]
		logger := logrus.WithFields(logrus.Fields{
			"job":            p.Name,
			"previous-found": previousFound,
		})

		if p.Cron == "" {
			shouldTrigger := j.Complete() && now.Sub(j.Status.StartTime.Time) > p.GetInterval()
			logger = logger.WithField("should-trigger", shouldTrigger)
			if !previousFound || shouldTrigger {
				prowJob := pjutil.NewProwJob(pjutil.PeriodicSpec(p), p.Labels, p.Annotations)
				prowJob.Namespace = cfg.ProwJobNamespace
				logger.WithFields(pjutil.ProwJobFields(&prowJob)).Info("Triggering new run of interval periodic.")
				if err := prowJobClient.Create(context.TODO(), &prowJob); err != nil {
					errs = append(errs, err)
				}
			}
		} else if cronTriggers.Has(p.Name) {
			shouldTrigger := j.Complete()
			logger = logger.WithField("should-trigger", shouldTrigger)
			if !previousFound || shouldTrigger {
				prowJob := pjutil.NewProwJob(pjutil.PeriodicSpec(p), p.Labels, p.Annotations)
				prowJob.Namespace = cfg.ProwJobNamespace
				logger.WithFields(pjutil.ProwJobFields(&prowJob)).Info("Triggering new run of cron periodic.")
				if err := prowJobClient.Create(context.TODO(), &prowJob); err != nil {
					errs = append(errs, err)
				}
			} else {
				logger.WithFields(logrus.Fields{
					"previous-found": previousFound,
					"should-trigger": shouldTrigger,
					"name":           p.Name,
					"job":            p.JobBase.Name,
				}).Info("skipping cron periodic")
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to create %d prowjobs: %v", len(errs), errs)
	}

	return nil
}
