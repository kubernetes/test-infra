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
	"crypto/tls"
	"crypto/x509"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/NYTimes/gziphandler"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/labels"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/jenkins"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/logrusutil"
	m "k8s.io/test-infra/prow/metrics"
)

type options struct {
	configPath    string
	jobConfigPath string
	selector      string
	totURL        string
	deckURL       string

	jenkinsURL             string
	jenkinsUserName        string
	jenkinsTokenFile       string
	jenkinsBearerTokenFile string
	certFile               string
	keyFile                string
	caCertFile             string
	csrfProtect            bool

	githubEndpoint  flagutil.Strings
	githubTokenFile string
	dryRun          bool
}

func (o *options) Validate() error {
	if o.jenkinsTokenFile == "" && o.jenkinsBearerTokenFile == "" {
		return errors.New("either --jenkins-token-file or --jenkins-bearer-token-file must be set")
	} else if o.jenkinsTokenFile != "" && o.jenkinsBearerTokenFile != "" {
		return errors.New("only one of --jenkins-token-file or --jenkins-bearer-token-file can be set")
	}

	var transportSecretsProvided int
	if o.certFile == "" {
		transportSecretsProvided = transportSecretsProvided + 1
	}
	if o.keyFile == "" {
		transportSecretsProvided = transportSecretsProvided + 1
	}
	if o.caCertFile == "" {
		transportSecretsProvided = transportSecretsProvided + 1
	}
	if transportSecretsProvided != 0 && transportSecretsProvided != 3 {
		return errors.New("either --cert-file, --key-file, and --ca-cert-file must all be provided or none of them must be provided")
	}
	return nil
}

func gatherOptions() options {
	o := options{
		githubEndpoint: flagutil.NewStrings("https://api.github.com"),
	}
	flag.StringVar(&o.configPath, "config-path", "/etc/config/config.yaml", "Path to config.yaml.")
	flag.StringVar(&o.jobConfigPath, "job-config-path", "", "Path to prow job configs.")
	flag.StringVar(&o.selector, "label-selector", kube.EmptySelector, "Label selector to be applied in prowjobs. See https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors for constructing a label selector.")
	flag.StringVar(&o.totURL, "tot-url", "", "Tot URL")
	flag.StringVar(&o.deckURL, "deck-url", "", "Deck URL for read-only access to the cluster.")

	flag.StringVar(&o.jenkinsURL, "jenkins-url", "http://jenkins-proxy", "Jenkins URL")
	flag.StringVar(&o.jenkinsUserName, "jenkins-user", "jenkins-trigger", "Jenkins username")
	flag.StringVar(&o.jenkinsTokenFile, "jenkins-token-file", "", "Path to the file containing the Jenkins API token.")
	flag.StringVar(&o.jenkinsBearerTokenFile, "jenkins-bearer-token-file", "", "Path to the file containing the Jenkins API bearer token.")
	flag.StringVar(&o.certFile, "cert-file", "", "Path to a PEM-encoded certificate file.")
	flag.StringVar(&o.keyFile, "key-file", "", "Path to a PEM-encoded key file.")
	flag.StringVar(&o.caCertFile, "ca-cert-file", "", "Path to a PEM-encoded CA certificate file.")
	flag.BoolVar(&o.csrfProtect, "csrf-protect", false, "Request a CSRF protection token from Jenkins that will be used in all subsequent requests to Jenkins.")

	flag.Var(&o.githubEndpoint, "github-endpoint", "GitHub's API endpoint.")
	flag.StringVar(&o.githubTokenFile, "github-token-file", "/etc/github/oauth", "Path to the file containing the GitHub OAuth token.")
	flag.BoolVar(&o.dryRun, "dry-run", true, "Whether or not to make mutating API calls to GitHub/Kubernetes/Jenkins.")
	flag.Parse()
	return o
}

func main() {
	o := gatherOptions()
	if err := o.Validate(); err != nil {
		logrus.Fatalf("Invalid options: %v", err)
	}
	logrus.SetFormatter(
		logrusutil.NewDefaultFieldsFormatter(nil, logrus.Fields{"component": "jenkins-operator"}),
	)

	if _, err := labels.Parse(o.selector); err != nil {
		logrus.WithError(err).Fatal("Error parsing label selector.")
	}

	configAgent := &config.Agent{}
	if err := configAgent.Start(o.configPath, o.jobConfigPath); err != nil {
		logrus.WithError(err).Fatal("Error starting config agent.")
	}

	kc, err := kube.NewClientInCluster(configAgent.Config().ProwJobNamespace)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting kube client.")
	}

	ac := &jenkins.AuthConfig{
		CSRFProtect: o.csrfProtect,
	}

	var tokens []string
	tokens = append(tokens, o.githubTokenFile)

	if o.jenkinsTokenFile != "" {
		tokens = append(tokens, o.jenkinsTokenFile)
	}

	if o.jenkinsBearerTokenFile != "" {
		tokens = append(tokens, o.jenkinsBearerTokenFile)
	}

	// Start the secret agent.
	secretAgent := &config.SecretAgent{}
	if err := secretAgent.Start(tokens); err != nil {
		logrus.WithError(err).Fatal("Error starting secrets agent.")
	}

	if o.jenkinsTokenFile != "" {
		ac.Basic = &jenkins.BasicAuthConfig{
			User:     o.jenkinsUserName,
			GetToken: secretAgent.GetTokenGenerator(o.jenkinsTokenFile),
		}
	} else if o.jenkinsBearerTokenFile != "" {
		ac.BearerToken = &jenkins.BearerTokenAuthConfig{
			GetToken: secretAgent.GetTokenGenerator(o.jenkinsBearerTokenFile),
		}
	}
	var tlsConfig *tls.Config
	if o.certFile != "" && o.keyFile != "" {
		config, err := loadCerts(o.certFile, o.keyFile, o.caCertFile)
		if err != nil {
			logrus.WithError(err).Fatalf("Could not read certificate files.")
		}
		tlsConfig = config
	}
	metrics := jenkins.NewMetrics()
	jc, err := jenkins.NewClient(o.jenkinsURL, o.dryRun, tlsConfig, ac, nil, metrics.ClientMetrics)
	if err != nil {
		logrus.WithError(err).Fatalf("Could not setup Jenkins client.")
	}

	// Check if github endpoint has a valid url.
	for _, ep := range o.githubEndpoint.Strings() {
		_, err = url.ParseRequestURI(ep)
		if err != nil {
			logrus.WithError(err).Fatalf("Invalid --endpoint URL %q.", ep)
		}
	}

	var ghc *github.Client
	if o.dryRun {
		ghc = github.NewDryRunClient(secretAgent.GetTokenGenerator(o.githubTokenFile), o.githubEndpoint.Strings()...)
		kc = kube.NewFakeClient(o.deckURL)
	} else {
		ghc = github.NewClient(secretAgent.GetTokenGenerator(o.githubTokenFile), o.githubEndpoint.Strings()...)
	}

	c, err := jenkins.NewController(kc, jc, ghc, nil, configAgent, o.totURL, o.selector)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to instantiate Jenkins controller.")
	}

	// Push metrics to the configured prometheus pushgateway endpoint.
	pushGateway := configAgent.Config().PushGateway
	if pushGateway.Endpoint != "" {
		go m.PushMetrics("jenkins-operator", pushGateway.Endpoint, pushGateway.Interval)
	}
	// Serve Jenkins logs here and proxy deck to use this endpoint
	// instead of baking agent-specific logic in deck. This func also
	// serves prometheus metrics.
	go serve(jc)
	// gather metrics for the jobs handled by the jenkins controller.
	go gather(c)

	tick := time.Tick(30 * time.Second)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	for {
		select {
		case <-tick:
			start := time.Now()
			if err := c.Sync(); err != nil {
				logrus.WithError(err).Error("Error syncing.")
			}
			duration := time.Since(start)
			logrus.WithField("duration", fmt.Sprintf("%v", duration)).Info("Synced")
			metrics.ResyncPeriod.Observe(duration.Seconds())
		case <-sig:
			logrus.Info("Jenkins operator is shutting down...")
			return
		}
	}
}

func loadCerts(certFile, keyFile, caCertFile string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	if caCertFile != "" {
		caCert, err := ioutil.ReadFile(caCertFile)
		if err != nil {
			return nil, err
		}
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)
		tlsConfig.RootCAs = caCertPool
	}

	tlsConfig.BuildNameToCertificate()
	return tlsConfig, nil
}

// serve starts a http server and serves Jenkins logs
// and prometheus metrics. Meant to be called inside
// a goroutine.
func serve(jc *jenkins.Client) {
	http.Handle("/", gziphandler.GzipHandler(handleLog(jc)))
	http.Handle("/metrics", promhttp.Handler())
	logrus.WithError(http.ListenAndServe(":8080", nil)).Fatal("ListenAndServe returned.")
}

// gather metrics from the jenkins controller.
// Meant to be called inside a goroutine.
func gather(c *jenkins.Controller) {
	tick := time.Tick(30 * time.Second)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	for {
		select {
		case <-tick:
			start := time.Now()
			c.SyncMetrics()
			logrus.WithField("metrics-duration", fmt.Sprintf("%v", time.Since(start))).Debug("Metrics synced")
		case <-sig:
			logrus.Debug("Jenkins operator gatherer is shutting down...")
			return
		}
	}
}
