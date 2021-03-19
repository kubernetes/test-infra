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
	"flag"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	prowv1 "k8s.io/test-infra/prow/client/clientset/versioned/typed/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/config/secret"
	"k8s.io/test-infra/prow/crier/reporters/pubsub"
	"k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/interrupts"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/metrics"
	"k8s.io/test-infra/prow/pubsub/subscriber"
)

var (
	flagOptions *options
)

type options struct {
	client         flagutil.KubernetesOptions
	port           int
	pushSecretFile string

	configPath                 string
	jobConfigPath              string
	supplementalProwConfigDirs flagutil.Strings
	pluginConfig               string

	dryRun                 bool
	gracePeriod            time.Duration
	instrumentationOptions flagutil.InstrumentationOptions
}

type kubeClient struct {
	client prowv1.ProwJobInterface
	dryRun bool
}

func (c *kubeClient) Create(ctx context.Context, job *prowapi.ProwJob, o metav1.CreateOptions) (*prowapi.ProwJob, error) {
	if c.dryRun {
		return job, nil
	}
	return c.client.Create(ctx, job, o)
}

func init() {
	flagOptions = &options{}
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	fs.IntVar(&flagOptions.port, "port", 80, "HTTP Port.")
	fs.StringVar(&flagOptions.pushSecretFile, "push-secret-file", "", "Path to Pub/Sub Push secret file.")

	fs.StringVar(&flagOptions.configPath, "config-path", "/etc/config/config.yaml", "Path to config.yaml.")
	fs.StringVar(&flagOptions.jobConfigPath, "job-config-path", "", "Path to prow job configs.")
	fs.Var(&flagOptions.supplementalProwConfigDirs, "supplemental-prow-config-dir", "An additional directory from which to load prow configs. Can be used for config sharding but only supports a subset of the config. The flag can be passed multiple times.")

	fs.BoolVar(&flagOptions.dryRun, "dry-run", true, "Dry run for testing. Uses API tokens but does not mutate.")
	fs.DurationVar(&flagOptions.gracePeriod, "grace-period", 180*time.Second, "On shutdown, try to handle remaining events for the specified duration. ")

	flagOptions.client.AddFlags(fs)
	flagOptions.instrumentationOptions.AddFlags(fs)

	fs.Parse(os.Args[1:])
}

func main() {
	logrusutil.ComponentInit()

	configAgent := &config.Agent{}
	if err := configAgent.Start(flagOptions.configPath, flagOptions.jobConfigPath, flagOptions.supplementalProwConfigDirs.Strings()); err != nil {
		logrus.WithError(err).Fatal("Error starting config agent.")
	}

	var tokenGenerator func() []byte
	if flagOptions.pushSecretFile != "" {
		var tokens []string
		tokens = append(tokens, flagOptions.pushSecretFile)

		secretAgent := &secret.Agent{}
		if err := secretAgent.Start(tokens); err != nil {
			logrus.WithError(err).Fatal("Error starting secrets agent.")
		}
		tokenGenerator = secretAgent.GetTokenGenerator(flagOptions.pushSecretFile)
	}

	prowjobClient, err := flagOptions.client.ProwJobClient(configAgent.Config().ProwJobNamespace, flagOptions.dryRun)
	if err != nil {
		logrus.WithError(err).Fatal("unable to create prow job client")
	}
	kubeClient := &kubeClient{
		client: prowjobClient,
		dryRun: flagOptions.dryRun,
	}

	promMetrics := subscriber.NewMetrics()

	defer interrupts.WaitForGracefulShutdown()

	// Expose prometheus metrics
	metrics.ExposeMetrics("sub", configAgent.Config().PushGateway, flagOptions.instrumentationOptions.MetricsPort)

	s := &subscriber.Subscriber{
		ConfigAgent:   configAgent,
		Metrics:       promMetrics,
		ProwJobClient: kubeClient,
		Reporter:      pubsub.NewReporter(configAgent.Config),
	}

	// Return 200 on / for health checks.
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {})

	// Setting up Push Server
	logrus.Info("Setting up Push Server")
	pushServer := &subscriber.PushServer{
		Subscriber:     s,
		TokenGenerator: tokenGenerator,
	}
	http.Handle("/push", pushServer)

	// Setting up Pull Server
	logrus.Info("Setting up Pull Server")
	pullServer := subscriber.NewPullServer(s)
	interrupts.Run(func(ctx context.Context) {
		if err := pullServer.Run(ctx); err != nil {
			logrus.WithError(err).Error("Failed to run Pull Server")
		}
	})

	httpServer := &http.Server{Addr: ":" + strconv.Itoa(flagOptions.port)}
	interrupts.ListenAndServe(httpServer, flagOptions.gracePeriod)
}
