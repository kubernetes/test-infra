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
	configflagutil "k8s.io/test-infra/prow/flagutil/config"
	"k8s.io/test-infra/prow/git/v2"
	"k8s.io/test-infra/prow/interrupts"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/metrics"
	"k8s.io/test-infra/prow/pjutil/pprof"
	"k8s.io/test-infra/prow/pubsub/subscriber"
)

var (
	flagOptions *options
)

type options struct {
	client         flagutil.KubernetesOptions
	github         flagutil.GitHubOptions
	port           int
	pushSecretFile string

	config       configflagutil.ConfigOptions
	pluginConfig string

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
	flagOptions = &options{config: configflagutil.ConfigOptions{ConfigPath: "/etc/config/config.yaml"}}
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	fs.IntVar(&flagOptions.port, "port", 80, "HTTP Port.")
	fs.StringVar(&flagOptions.pushSecretFile, "push-secret-file", "", "Path to Pub/Sub Push secret file.")

	fs.BoolVar(&flagOptions.dryRun, "dry-run", true, "Dry run for testing. Uses API tokens but does not mutate.")
	fs.DurationVar(&flagOptions.gracePeriod, "grace-period", 180*time.Second, "On shutdown, try to handle remaining events for the specified duration. ")

	flagOptions.config.AddFlags(fs)
	flagOptions.client.AddFlags(fs)
	flagOptions.github.AddFlags(fs)
	flagOptions.instrumentationOptions.AddFlags(fs)

	fs.Parse(os.Args[1:])
}

func main() {
	logrusutil.ComponentInit()

	configAgent, err := flagOptions.config.ConfigAgent()
	if err != nil {
		logrus.WithError(err).Fatal("Error starting config agent.")
	}

	var tokens []string
	if flagOptions.pushSecretFile != "" {
		tokens = append(tokens, flagOptions.pushSecretFile)
	}
	if flagOptions.github.TokenPath != "" {
		tokens = append(tokens, flagOptions.github.TokenPath)
	}
	if err := secret.Add(tokens...); err != nil {
		logrus.WithError(err).Fatal("failed to start secret agent")
	}
	tokenGenerator := secret.GetTokenGenerator(flagOptions.pushSecretFile)

	var gitClientFactory git.ClientFactory
	if flagOptions.github.TokenPath != "" {
		gitClient, err := flagOptions.github.GitClient(flagOptions.dryRun)
		if err != nil {
			logrus.WithError(err).Fatal("Error getting Git client.")
		}
		gitClientFactory = git.ClientFactoryFrom(gitClient)
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

	// Expose prometheus and pprof metrics
	metrics.ExposeMetrics("sub", configAgent.Config().PushGateway, flagOptions.instrumentationOptions.MetricsPort)
	pprof.Instrument(flagOptions.instrumentationOptions)

	s := &subscriber.Subscriber{
		ConfigAgent:   configAgent,
		Metrics:       promMetrics,
		ProwJobClient: kubeClient,
		GitClient:     config.NewInRepoConfigGitCache(gitClientFactory),
		Reporter:      pubsub.NewReporter(configAgent.Config), // reuse crier reporter
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
