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
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"time"

	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"

	"k8s.io/test-infra/pkg/flagutil"
	"k8s.io/test-infra/prow/config"
	prowflagutil "k8s.io/test-infra/prow/flagutil"
	configflagutil "k8s.io/test-infra/prow/flagutil/config"
	"k8s.io/test-infra/prow/gangway"
	"k8s.io/test-infra/prow/interrupts"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/metrics"
	"k8s.io/test-infra/prow/pjutil"
)

// Empty string represents the overall health of all gRPC services. See
// https://github.com/grpc/grpc/blame/e699e0135e4d59e3763e7bdea5fff002cc2efab3/doc/health-checking.md#L64.
const serviceNameForHealthCheckGrpc = ""

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

	if o.port == o.instrumentationOptions.HealthPort {
		errs = append(errs, fmt.Errorf("both the gRPC port and health port are using the same port number %d", o.port))
	}

	return utilerrors.NewAggregate(errs)
}

// interruptableServer is a wrapper type around the gRPC server, so that we can
// pass it along to our own interrupts package.
type interruptableServer struct {
	grpcServer *grpc.Server
	listener   net.Listener
	port       int
}

// Shutdown shuts down the inner gRPC server as gracefully as possible, by first
// invoking GracefulStop() on it. This gives the server time to try to handle
// things gracefully internally. However if it takes too long (if the parent
// context cancels us), we forcefully kill the server by calling Stop(). Stop()
// interrupts GracefulStop() (see
// https://pkg.go.dev/google.golang.org/grpc#Server.Stop).
func (s *interruptableServer) Shutdown(ctx context.Context) error {

	gracefulStopFinished := make(chan struct{})

	go func() {
		s.grpcServer.GracefulStop()
		close(gracefulStopFinished)
	}()

	select {
	case <-gracefulStopFinished:
		return nil
	case <-ctx.Done():
		s.grpcServer.Stop()
		return ctx.Err()
	}
}

func (s *interruptableServer) ListenAndServe() error {
	logrus.Infof("serving gRPC on port %d", s.port)
	return s.grpcServer.Serve(s.listener)
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

	// If we are provided credentials for Git hosts, use them. These credentials
	// hold per-host information in them so it's safe to set them globally.
	if o.cookiefilePath != "" {
		cmd := exec.Command("git", "config", "--global", "http.cookiefile", o.cookiefilePath)
		if err := cmd.Run(); err != nil {
			logrus.WithError(err).Fatal("unable to set cookiefile")
		}
	}

	gitClient, err := o.github.GitClientFactory(o.cookiefilePath, &o.config.InRepoConfigCacheDirBase, o.dryRun, false)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting Git client.")
	}

	cacheGetter, err := config.NewInRepoConfigCache(o.config.InRepoConfigCacheSize, configAgent, gitClient)
	if err != nil {
		logrus.WithError(err).Fatal("Error creating InRepoConfigCacheGetter.")
	}
	metrics.ExposeMetrics("gangway", configAgent.Config().PushGateway, o.instrumentationOptions.MetricsPort)

	// Start serving liveness endpoint /healthz.
	healthHTTP := pjutil.NewHealthOnPort(o.instrumentationOptions.HealthPort)

	gw := gangway.Gangway{
		ConfigAgent:       configAgent,
		ProwJobClient:     prowjobClient,
		InRepoConfigCache: cacheGetter,
	}

	lis, err := net.Listen("tcp", ":"+strconv.Itoa(o.port))
	if err != nil {
		logrus.WithError(err).Fatal("failed to set up tcp connection")
	}

	// Create a new gRPC (empty) server, and wire it up to act as a "ProwServer"
	// as defined in the auto-generated gangway_grpc.pb.go file. Also inject an
	// interceptor for collecting Prometheus metrics for all unary gRPC
	// requests.
	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(grpc_prometheus.UnaryServerInterceptor),
	)
	gangway.RegisterProwServer(grpcServer, &gw)
	grpc_prometheus.Register(grpcServer)

	// Create gRPC health check endpoint and add it to our server. This can be
	// used by the ESPv2 proxy container if Gangway is deployed for Cloud
	// Endpoints.
	//
	// Note that this just configures our gRPC server to be able to respond to
	// native gRPC health check requests. The actual serving of these health
	// check requests will only happen when we finally call
	// interrupts.ListenAndServe() down below.
	healthcheckGrpc := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthcheckGrpc)
	healthcheckGrpc.SetServingStatus(serviceNameForHealthCheckGrpc, healthpb.HealthCheckResponse_SERVING)

	// Register reflection service on gRPC server. This enables testing through
	// clients that don't have the generated stubs baked in, such as grpcurl.
	reflection.Register(grpcServer)

	s := &interruptableServer{
		grpcServer: grpcServer,
		listener:   lis,
		port:       o.port,
	}

	// Start serving readiness endpoint /healthz/ready. Note that this is a
	// workaround for older Kubernetes clusters (older than K8s 1.24) that do
	// not support native gRPC health checks.
	healthHTTP.ServeReady()

	// Start serving gRPC requests! Note that ListenAndServe() does not block,
	// while WaitForGracefulShutdown() does block.
	interrupts.ListenAndServe(s, o.gracePeriod)
	interrupts.WaitForGracefulShutdown()
	logrus.Info("Ended gracefully")
}
