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

package main

import (
	"context"
	"flag"
	"net"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/test-infra/pkg/flagutil"
	prowcr "k8s.io/test-infra/prow/apis/prowjobs/v1"
	prowv1 "k8s.io/test-infra/prow/client/clientset/versioned/typed/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	prowflagutil "k8s.io/test-infra/prow/flagutil"
	configflagutil "k8s.io/test-infra/prow/flagutil/config"
	"k8s.io/test-infra/prow/gangway"
	"k8s.io/test-infra/prow/logrusutil"
)

type options struct {
	client         prowflagutil.KubernetesOptions
	github         prowflagutil.GitHubOptions
	port           int
	cookiefilePath string

	config configflagutil.ConfigOptions

	dryRun                 bool
	gracePeriod            time.Duration
	instrumentationOptions prowflagutil.InstrumentationOptions
}

func gatherOptions(fs *flag.FlagSet, args ...string) options {
	var o options
	fs.IntVar(&o.port, "port", 32000, "TCP port for gRPC.")
	fs.BoolVar(&o.dryRun, "dry-run", true, "Dry run for testing. Uses API tokens but does not mutate.")
	fs.DurationVar(&o.gracePeriod, "grace-period", 180*time.Second, "On shutdown, try to handle remaining events for the specified duration. ")
	fs.StringVar(&o.cookiefilePath, "cookiefile", "", "Path to git http.cookiefile, leave empty for github or anonymous")
	for _, group := range []flagutil.OptionGroup{&o.client, &o.github, &o.instrumentationOptions, &o.config} {
		group.AddFlags(fs)
	}

	fs.Parse(args)

	return o
}

func (o *options) validate() error {
	var errs []error
	for _, group := range []flagutil.OptionGroup{&o.client, &o.github, &o.instrumentationOptions, &o.config} {
		if err := group.Validate(o.dryRun); err != nil {
			errs = append(errs, err)
		}
	}

	return utilerrors.NewAggregate(errs)
}

type kubeClient struct {
	client prowv1.ProwJobInterface
	dryRun bool
}

// Create creates a Prow Job CR in the Kubernetes cluster (Prow service cluster).
func (c *kubeClient) Create(ctx context.Context, job *prowcr.ProwJob, o metav1.CreateOptions) (*prowcr.ProwJob, error) {
	if c.dryRun {
		return job, nil
	}
	return c.client.Create(ctx, job, o)
}

func main() {
	logrusutil.ComponentInit()

	o := gatherOptions(flag.NewFlagSet(os.Args[0], flag.ExitOnError), os.Args[1:]...)
	if err := o.validate(); err != nil {
		logrus.WithError(err).Fatal("Invalid options")
	}

	configAgent, err := o.config.ConfigAgent()
	if err != nil {
		logrus.WithError(err).Fatal("Error starting config agent.")
	}

	prowjobClient, err := o.client.ProwJobClient(configAgent.Config().ProwJobNamespace, o.dryRun)
	if err != nil {
		logrus.WithError(err).Fatal("unable to create prow job client")
	}
	kubeClient := &kubeClient{
		client: prowjobClient,
		dryRun: o.dryRun,
	}

	// If we are provided credentials for Git hosts, use them. These credentials
	// hold per-host information in them so it's safe to set them globally.
	if o.cookiefilePath != "" {
		cmd := exec.Command("git", "config", "--global", "http.cookiefile", o.cookiefilePath)
		if err := cmd.Run(); err != nil {
			logrus.WithError(err).Fatal("unable to set cookiefile")
		}
	}

	gitClient, err := o.github.GitClientFactory(o.cookiefilePath, &o.config.InRepoConfigCacheDirBase, o.dryRun)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting Git client.")
	}
	cacheGetter, err := config.NewInRepoConfigCacheHandler(o.config.InRepoConfigCacheSize, configAgent, gitClient, o.config.InRepoConfigCacheCopies)
	if err != nil {
		logrus.WithError(err).Fatal("Error creating InRepoConfigCacheGetter.")
	}

	gw := gangway.Gangway{
		ConfigAgent:              configAgent,
		ProwJobClient:            kubeClient,
		InRepoConfigCacheHandler: cacheGetter,
	}

	logrus.Infof("serving gRPC on port %d", o.port)

	lis, err := net.Listen("tcp", ":"+strconv.Itoa(o.port))
	if err != nil {
		logrus.WithError(err).Fatal("failed to set up tcp connection")
	}
	grpcServer := grpc.NewServer()
	gangway.RegisterProwServer(grpcServer, &gw)
	// Register reflection service on gRPC server. This enables testing through
	// clients that don't have the generated stubs baked in, such as grpcurl.
	reflection.Register(grpcServer)
	if err := grpcServer.Serve(lis); err != nil {
		logrus.WithError(err).Fatal("failed to set up grpc server")
	}
}
